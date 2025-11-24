package usecase

import (
	"context"
	"fmt"
	"sync"
)

type CommandHandler func(ctx context.Context, payload interface{}) (interface{}, error)
type QueryHandler func(ctx context.Context, params interface{}) (interface{}, error)

type Dispatcher struct {
	cmdHandlers map[string]CommandHandler
	qryHandlers map[string]QueryHandler
	mu          sync.RWMutex
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		cmdHandlers: make(map[string]CommandHandler),
		qryHandlers: make(map[string]QueryHandler),
	}
}

func (d *Dispatcher) RegisterCommand(name string, handler CommandHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cmdHandlers[name] = handler
}

func (d *Dispatcher) RegisterQuery(name string, handler QueryHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.qryHandlers[name] = handler
}

func (d *Dispatcher) ExecuteCommand(ctx context.Context, name string, payload interface{}) (interface{}, error) {
	d.mu.RLock()
	handler, ok := d.cmdHandlers[name]
	d.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("command handler %s not registered", name)
	}
	return handler(ctx, payload)
}

func (d *Dispatcher) ExecuteQuery(ctx context.Context, name string, params interface{}) (interface{}, error) {
	d.mu.RLock()
	handler, ok := d.qryHandlers[name]
	d.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("query handler %s not registered", name)
	}
	return handler(ctx, params)
}
