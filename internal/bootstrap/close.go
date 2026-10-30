package bootstrap

import "context"

type resourceGroup []Storage

func (group resourceGroup) Ping(ctx context.Context) error {
	for _, resource := range group {
		if resource == nil {
			continue
		}
		if err := resource.Ping(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (group resourceGroup) Close() error {
	var first error
	for _, resource := range group {
		if resource == nil {
			continue
		}
		if err := resource.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
