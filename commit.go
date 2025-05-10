package ocfl

import (
	"context"
	"log/slog"
	"time"
)

// Commit represents an update to object.
type Commit struct {
	ID      string // required for new objects in storage roots without a layout.
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created         time.Time // time.Now is used, if not set
	Spec            Spec      // OCFL specification version for the new object version
	NewHEAD         int       // enforces new object version number
	AllowUnchanged  bool
	ContentPathFunc func(oldPaths []string) (newPaths []string)

	Logger *slog.Logger
}

type CommitPlan struct {
	NewInventory *Inventory   `json:"new_inventory"`
	Steps        []CommitStep `json:"steps"`
}

func (s *CommitPlan) Apply(ctx context.Context) error {
	for _, step := range s.Steps {
		if err := step.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *CommitPlan) append(step CommitStep) {
	s.Steps = append(s.Steps, step)
}

type CommitStep struct {
	Name string `json:"name"`
	//Async          []CommitStep `json:"async,omitempty"`
	Err  string `json:"error,omitempty"`
	Done bool   `json:"done,omitempty"`
	//CompensateErr  string       `json:"compensate_error"`
	//CompensateDone bool         `json:"compensate_done"`

	run        func(ctx context.Context) error
	compensate func(ctx context.Context) error
}

func (step *CommitStep) Run(ctx context.Context) error {
	if step.run != nil {
		if err := step.run(ctx); err != nil {
			step.Err = err.Error()
			return err
		}
	}
	step.Done = true
	return nil
}

// Commit error wraps an error from a commit.
type CommitError struct {
	Err error // The wrapped error

	// Dirty indicates the object may be incomplete or invalid as a result of
	// the error.
	Dirty bool
}

func (c CommitError) Error() string {
	return c.Err.Error()
}

func (c CommitError) Unwrap() error {
	return c.Err
}
