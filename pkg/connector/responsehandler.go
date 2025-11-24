package connector

import (
	"sync"
	"time"
)

type responseHandler struct {
	client *MetaClient

	lock         sync.RWMutex
	editChannels map[string]chan *FBEditEvent
}

func newResponseHandler(client *MetaClient) responseHandler {
	return responseHandler{
		client:       client,
		editChannels: map[string]chan *FBEditEvent{},
	}
}

func (r *responseHandler) handleEdit(ev *FBEditEvent) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	if ch, ok := r.editChannels[ev.LSEditMessage.MessageID]; ok {
		select {
		case ch <- ev:
			return
		case <-time.After(1 * time.Second):
			r.client.Client.Logger.Warn().Msg("Dropped LSEditMessage from channel due to internal error")
			return
		}
	}
}

// subscribeToEdits returns a channel that will receive every
// LightSpeed edit event that is targeted at the given message ID. Any
// previous channel returned for the same message ID will stop
// receiving events. This is for simplicity based on the assumption
// that the user only makes one edit at a time to the same message.
func (r *responseHandler) subscribeToEdits(messageID string) chan *FBEditEvent {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.editChannels[messageID] = make(chan *FBEditEvent, 1)
	return r.editChannels[messageID]
}
