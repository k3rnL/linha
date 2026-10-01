// Package engine owns provisioning contracts; request management uses no Spark types.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"linha/server/internal/domain"
	"sync"
)

type Instance struct {
	ID, ContextID, Incarnation, Image string
	Kind, ResourceUID                 string
	Ready, Draining                   bool
	Condition                         string
}
type Adapter interface {
	Validate(domain.EngineSpec) error
	Ensure(context.Context, domain.BackendContext, Instance) error
	Observe(context.Context, Instance) (Instance, error)
	Drain(context.Context, Instance) error
	Stop(context.Context, Instance) error
}
type Registry map[string]Adapter

func (r Registry) Validate(s domain.EngineSpec) error {
	a, ok := r[s.Type]
	if !ok {
		return domain.Bad("unsupported engine")
	}
	return a.Validate(s)
}

// Fake is a deterministic adapter for controller conformance and local SDK development.
type Fake struct {
	mu        sync.Mutex
	instances map[string]Instance
}

func NewFake() *Fake { return &Fake{instances: map[string]Instance{}} }
func (f *Fake) Validate(s domain.EngineSpec) error {
	if s.Version != "1" {
		return domain.Bad("fake engine version must be 1")
	}
	var settings map[string]json.RawMessage
	if len(s.Settings) > 0 {
		if err := json.Unmarshal(s.Settings, &settings); err != nil {
			return domain.Bad("settings must be an object")
		}
	}
	if len(settings) > 0 {
		return domain.Bad("unknown fake engine settings")
	}
	return nil
}
func (f *Fake) Ensure(_ context.Context, c domain.BackendContext, i Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.instances[i.ID]; ok {
		if old.ContextID != i.ContextID || old.Image != i.Image {
			return fmt.Errorf("instance identity conflict")
		}
		return nil
	}
	i.Incarnation = domain.ID()
	i.Ready = true
	f.instances[i.ID] = i
	return nil
}
func (f *Fake) Observe(_ context.Context, i Instance) (Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.instances[i.ID]
	if !ok {
		return Instance{}, domain.NotFound
	}
	return v, nil
}
func (f *Fake) Drain(_ context.Context, i Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := f.instances[i.ID]
	v.Draining = true
	f.instances[i.ID] = v
	return nil
}
func (f *Fake) Stop(_ context.Context, i Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.instances, i.ID)
	return nil
}
