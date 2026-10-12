package controller

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const retiredCleanupInterval = time.Minute

// cleanup retains late-create recovery without polling every historical name.
// STOPPED contexts stay discoverable through existing resources. DEAD intents
// never become active again, so a new client/replacement uses different IDs.
func (c *Controller) cleanup(ctx context.Context) error {
	if time.Now().Before(c.nextCleanup) {
		return nil
	}
	c.nextCleanup = time.Now().Add(retiredCleanupInterval)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resources, err := c.Adapter.ListResources(ctx)
	if err != nil {
		return fmt.Errorf("retired resource inventory: %w", err)
	}
	var failures []error
	for _, r := range resources {
		// Match both context and intent kind before claiming ownership. Unknown
		// resources (including another Linha database's) must remain untouched.
		tag, err := c.Pool.Exec(ctx, `UPDATE linha_contexts c
SET reconcile_epoch=CASE WHEN reconcile_owner=$2 AND reconcile_until>clock_timestamp() THEN reconcile_epoch ELSE reconcile_epoch+1 END,
    reconcile_owner=$2,reconcile_until=clock_timestamp()+interval '30 seconds'
WHERE c.id=$1 AND (reconcile_until<=clock_timestamp() OR reconcile_owner=$2)
AND c.spec->'engine'->>'type'='spark'
AND EXISTS(SELECT 1 FROM linha_instances i WHERE i.id=$3 AND i.context_id=c.id AND i.resource_kind=$4 AND i.state='DEAD')`, r.ContextID, c.ID, r.InstanceID, r.InstanceKind)
		if err != nil {
			return errors.Join(append(failures, fmt.Errorf("authorize retired resource cleanup: %w", err))...)
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		// The call has less time remaining than the renewed ownership lease.
		// UID preconditions protect against a resource changing after the LIST.
		if err = c.Adapter.DeleteResource(ctx, r); err != nil {
			failures = append(failures, fmt.Errorf("delete retired resource %s for instance %s: %w", r.Kind, r.InstanceID, err))
		}
	}
	return errors.Join(failures...)
}
