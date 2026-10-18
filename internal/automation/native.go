package automation

import (
	"context"
	"errors"
	"strings"

	"prisme.local/platform/internal/coordination"
	"prisme.local/platform/internal/deployment"
)

type Native struct{ Deployments *deployment.Engine }

func (n *Native) Validate(c Config) error {
	if n.Deployments == nil || n.Deployments.Config.CoordinationRoot == "" {
		return ErrInvalid
	}
	for _, t := range c.Targets {
		found := false
		for _, m := range n.Deployments.Config.Targets {
			if m.ID != t.Deployment {
				continue
			}
			if m.Server != t.Server || m.Project != t.Project || m.Environment != t.Environment {
				return ErrInvalid
			}
			for _, s := range m.Services {
				if s.Name == t.Service && s.Stateless {
					found = true
				}
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}
func (n *Native) Reconcile(ctx context.Context, t Target, run string) error {
	state, err := coordination.Read(n.Deployments.Config.CoordinationRoot, t.Server)
	if err != nil || state.Active || state.Suspended {
		return ErrBlocked
	}
	return n.Deployments.ReconcileAutomation(ctx, t.Deployment, run)
}
func (n *Native) Check(ctx context.Context, t Target) error {
	p, err := n.Deployments.ActivePlan(ctx, t.Deployment)
	if err != nil {
		return ErrBlocked
	}
	err = n.Deployments.Driver.Verify(ctx, p)
	if errors.Is(err, deployment.ErrHealth) {
		return ErrHealth
	}
	if err != nil {
		return ErrBlocked
	}
	return nil
}
func (n *Native) Execute(ctx context.Context, t Target, s Step, key string) (map[string]any, error) {
	if s.Action == "restart" {
		err := n.Deployments.RestartService(ctx, t.Deployment, t.Service)
		if errors.Is(err, deployment.ErrPrecondition) {
			return nil, ErrBlocked
		}
		if err != nil {
			return nil, ErrUncertain
		}
		return map[string]any{"service": t.Service, "target": t.Deployment}, nil
	}
	if s.Action != "rollback" {
		return nil, ErrBlocked
	}
	j, err := n.Deployments.Prepare(ctx, deployment.Request{Target: t.Deployment, Release: s.Release, Rollback: true}, key)
	if err != nil {
		return nil, ErrBlocked
	}
	parts := strings.Split(key, "_")
	if len(parts) != 3 {
		return nil, ErrBlocked
	}
	if _, err = n.Deployments.DB.ExecContext(ctx, `UPDATE deployment_jobs SET automation_run_id=$2 WHERE id=$1 AND (automation_run_id='' OR automation_run_id=$2)`, j.ID, parts[1]); err != nil {
		return nil, ErrBlocked
	}
	if len(j.Plan.Blockers) != 0 || j.Plan.Manifest.Migration == "incompatible" {
		return map[string]any{"deployment_job": j.ID}, ErrBlocked
	}
	if j.State == "awaiting_approval" {
		next, decideErr := n.Deployments.Decide(ctx, j.ID, "approve", t.Deployment, "automation-worker", j.Revision)
		if decideErr != nil {
			return map[string]any{"deployment_job": j.ID}, ErrBlocked
		}
		j = next
	}
	if j.State == "ready" {
		if err = n.Deployments.RunTarget(ctx, t.Deployment); err != nil {
			return map[string]any{"deployment_job": j.ID}, ErrUncertain
		}
	}
	j, err = n.Deployments.Job(ctx, j.ID)
	if err != nil || j.State != "succeeded" {
		return map[string]any{"deployment_job": j.ID}, ErrUncertain
	}
	return map[string]any{"deployment_job": j.ID, "release": s.Release}, nil
}
