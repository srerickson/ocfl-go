package mutate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl/backend"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/object"
)

type memoryStageManager struct {
	// backend storage for staged content
	fsys backend.Interface
	// directory in fsys where are all stages are created
	root   string
	stages map[string]*objStage
	logger logr.Logger
}

func newMemoryStageManager(fsys backend.Interface, root string, logger logr.Logger) *memoryStageManager {
	return &memoryStageManager{fsys: fsys,
		root:   root,
		stages: map[string]*objStage{},
		logger: logger,
	}
}

func (m *memoryStageManager) StageId(objectId string, v object.VNum) string {
	byts := sha256.Sum256([]byte(objectId + v.String()))
	return hex.EncodeToString(byts[:])

}
func (m *memoryStageManager) Stages(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m.stages))
	for k := range m.stages {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *memoryStageManager) GetStage(ctx context.Context, id string) (*objStage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if stage, ok := m.stages[id]; ok {
		return stage, nil
	}
	return nil, fmt.Errorf("stage does not exist")
}

func (m *memoryStageManager) SaveStage(ctx context.Context, stage *objStage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stageId := m.StageId(stage.objectID, stage.versionNum)
	m.stages[stageId] = stage
	return nil
}

// inititalize an empty stage for a new object
// TODO add logger to stage
func (m *memoryStageManager) InitStage(ctx context.Context, id string) (*objStage, error) {
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &objStage{
		fsys:            m.fsys,
		stageRoot:       path.Join(m.root, m.StageId(id, object.V1)),
		objectID:        id,
		versionNum:      object.V1,
		digestAlgorithm: digest.SHA512,
	}, nil
}
