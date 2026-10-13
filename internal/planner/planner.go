package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"air-automation/internal/action"
	"air-automation/internal/site"
)

type Request struct {
	Task string
	Site *site.Profile
}

type Planner interface {
	Plan(ctx context.Context, req Request) (action.Plan, error)
}

type File struct {
	Path string
}

func (f File) Plan(_ context.Context, req Request) (action.Plan, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return action.Plan{}, fmt.Errorf("read plan: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan action.Plan
	if err := decoder.Decode(&plan); err != nil {
		return action.Plan{}, fmt.Errorf("parse plan %s: %w", f.Path, err)
	}
	if plan.Task == "" {
		plan.Task = req.Task
	}
	return plan, nil
}
