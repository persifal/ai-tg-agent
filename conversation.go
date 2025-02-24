package main

import (
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

type (
	msg    = anthropic.MessageParam
	chatId = int64
)

type Conversation struct {
	history []msg
}

type InMemoryConvRepo struct {
	sync.RWMutex
	sessions map[chatId]*Conversation
}

func NewRepo() *InMemoryConvRepo {
	return &InMemoryConvRepo{
		sessions: make(map[chatId]*Conversation),
	}
}

func (r *InMemoryConvRepo) NewConversation(id chatId) (*Conversation, bool) {
	r.Lock()
	defer r.Unlock()
	v, ok := r.sessions[id]
	if ok {
		return v, false
	}
	conv := &Conversation{
		make([]msg, 0),
	}
	r.sessions[id] = conv
	return conv, true
}

func (r *InMemoryConvRepo) CloseConversation(id chatId) {
	r.Lock()
	defer r.Unlock()
	delete(r.sessions, id)
}

func (r *InMemoryConvRepo) Get(id chatId) (v *Conversation, ok bool) {
	r.RLock()
	defer r.RUnlock()
	v, ok = r.sessions[id]
	return
}

func (r *InMemoryConvRepo) AddMessage(id chatId, msg msg) {
	r.RLock()
	defer r.RUnlock()
	c, ok := r.sessions[id]
	if ok {
		c.history = append(c.history, msg)
	}
}

func (r *InMemoryConvRepo) Exists(id chatId) bool {
	r.RLock()
	defer r.RUnlock()
	_, ok := r.sessions[id]
	return ok
}
