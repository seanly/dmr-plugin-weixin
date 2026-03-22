package main

import (
	"context"
	"log"
	"strings"
	"time"
)

const (
	defaultLongPollTimeoutMS = 35_000
	maxConsecutiveFailures   = 3
	backoffDelay             = 30 * time.Second
	retryDelay               = 2 * time.Second
	sessionPauseDuration     = time.Hour
)

func (p *WeixinPlugin) monitorLoop(ctx context.Context) {
	buf := p.loadSyncBuf()
	nextTimeout := time.Duration(defaultLongPollTimeoutMS) * time.Millisecond
	consecutive := 0
	var pauseUntil time.Time

	for {
		select {
		case <-ctx.Done():
			log.Printf("weixin: monitor stopped")
			return
		default:
		}

		if !pauseUntil.IsZero() {
			if time.Now().Before(pauseUntil) {
				sleep := time.Until(pauseUntil)
				log.Printf("weixin: session pause %v remaining", sleep)
				select {
				case <-ctx.Done():
					return
				case <-time.After(sleep):
				}
				continue
			}
			pauseUntil = time.Time{}
		}

		pollCtx, cancel := context.WithTimeout(ctx, nextTimeout)
		resp, err := p.getUpdates(pollCtx, buf, nextTimeout)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			consecutive++
			log.Printf("weixin: getUpdates error (%d/%d): %v", consecutive, maxConsecutiveFailures, err)
			if consecutive >= maxConsecutiveFailures {
				consecutive = 0
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDelay):
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			}
			continue
		}

		apiErr := (resp.Ret != 0) || (resp.Errcode != 0)
		if apiErr {
			if resp.Errcode == sessionExpiredErrcode || resp.Ret == sessionExpiredErrcode {
				log.Printf("weixin: session expired (errcode %d), pausing 1h", sessionExpiredErrcode)
				pauseUntil = time.Now().Add(sessionPauseDuration)
				consecutive = 0
				continue
			}
			consecutive++
			log.Printf("weixin: getUpdates ret=%d errcode=%d msg=%q (%d/%d)", resp.Ret, resp.Errcode, resp.Errmsg, consecutive, maxConsecutiveFailures)
			if consecutive >= maxConsecutiveFailures {
				consecutive = 0
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDelay):
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			}
			continue
		}
		consecutive = 0

		if resp.LongpollingTimeoutMs > 0 {
			nextTimeout = time.Duration(resp.LongpollingTimeoutMs) * time.Millisecond
		}
		if resp.GetUpdatesBuf != "" {
			p.saveSyncBuf(resp.GetUpdatesBuf)
			buf = resp.GetUpdatesBuf
		}

		for _, full := range resp.Msgs {
			p.handleInboundMessage(ctx, full)
		}
	}
}

func (p *WeixinPlugin) handleInboundMessage(ctx context.Context, full weixinMessage) {
	if strings.TrimSpace(full.GroupID) != "" {
		log.Printf("weixin: skip group message group_id=%q", full.GroupID)
		return
	}
	if full.MessageType == msgTypeBot {
		return
	}
	peerID := strings.TrimSpace(full.FromUserID)
	if peerID == "" {
		return
	}
	if tok := full.inboundContextToken(); tok != "" {
		p.tokens.set(peerID, tok)
		p.prefetchOutboundSession(ctx, peerID, tok)
	}
	if sid := full.inboundSessionID(); sid != "" {
		p.rememberSessionForPeer(peerID, sid)
	}

	dkey := dedupKeyForMessage(full)
	if dkey != "" && p.dedup != nil && p.dedup.isDuplicate(dkey) {
		log.Printf("weixin: dedup skip %s peer=%q", dkey, peerID)
		return
	}

	body := bodyFromItemList(full.ItemList)
	if strings.TrimSpace(body) == "" {
		body = "[empty or non-text message]"
	}

	if p.approver != nil && p.approver.tryResolveP2P(peerID, body) {
		log.Printf("weixin: approval reply consumed peer=%q", peerID)
		return
	}

	if !isAllowedSender(p.cfg.AllowFrom, peerID) {
		log.Printf("weixin: sender not allowed peer=%q", peerID)
		return
	}

	tape := tapeNameForP2P(peerID)
	jobSid := strings.TrimSpace(full.inboundSessionID())
	if jobSid == "" {
		jobSid = strings.TrimSpace(p.sessionIDForPeer(peerID))
	}
	job := &inboundJob{
		QueueKey:     tape,
		TapeName:     tape,
		PeerID:       peerID,
		Content:      body,
		TriggerMsgID: dkey,
		ContextToken: full.inboundContextToken(),
		SessionID:    jobSid,
	}

	if p.tryHandleDMRRestart(ctx, job, body) {
		return
	}

	if p.queues != nil {
		p.queues.enqueue(job)
	}
}
