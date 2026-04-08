package probe

import (
	"context"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/result"
)

type Probe interface {
	Run(ctx context.Context) result.Result
}

func BaseResult(check config.Check) result.Result {
	return result.Result{
		CheckID:    check.ID,
		CheckName:  check.Name,
		CheckGroup: check.Group,
		CheckKind:  check.Kind,
		CheckScope: check.Scope,
		Labels:     cloneLabels(check.Labels),
	}
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}
