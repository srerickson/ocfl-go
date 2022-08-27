package mutate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/checksum"
)

// Stages always use "content" as the content directory
const contentDir = "content"

var errDirtyState = errors.New("stage may be corrupt from previous error")

// objStage represents an in-progress and possibly incomplete version of an Object.
// in needs to be marshallable to json for later retrieval by a stage manager
type objStage struct {
	// Backend where stage content files are written
	fsys ocfl.WriteFS
	// the path to the staged object root in fsys.
	// Note: content are stored in stageRoot/version/content/...
	stageRoot string
	//object id
	objectID string
	// versionNum name for the new versionNum
	versionNum ocfl.VNum
	//OCFL ocfl.Number (if changing version)
	//OCFLVersion ocfl.Num
	// DigestAlgorithm (inherited from object if v >1 )
	digestAlgorithm digest.Alg
	// New version state
	Logical *digest.Map
	// Created time.Time
	Message string
	User    struct {
		Name    string
		Address string
	}

	// NewContent for new content files added to the stage
	NewContent *digest.Map
	// manifest from previous version (if v > 1)
	PrevContent *digest.Map
	// Stage state may be inconsistent due to error
	err error
}

func (s *objStage) Err() error {
	return s.err
}

func (s *objStage) ObjectID() string {
	return s.objectID
}

func (s *objStage) VersionNum() ocfl.VNum {
	return s.versionNum
}

func (s *objStage) DigestAlgorithm() digest.Alg {
	return s.digestAlgorithm
}

func (s *objStage) Backend() ocfl.WriteFS {
	return s.fsys
}

func (s *objStage) StageRoot() string {
	return s.stageRoot
}

// func (stage *Stage) SetMessage(m string) {
// 	stage.Version.Message = m
// }

// func (stage *Stage) SetUser(name, address string) {
// 	stage.Version.User.Name = name
// 	stage.Version.User.Address = address
// }

func (s *objStage) ContentDir() string {
	return path.Join(s.stageRoot, contentDir)
}

// FromFS uses all contents of dir in fsys as the state for stage
// inventory. Files in dir are digested, added to the stage state,
// added to the stage's content directory and included in the stage
// manifest.
func (s *objStage) FromFS(fsys fs.FS, dir string) error {
	if s.err != nil {
		return errDirtyState
	}
	// fs.FS rooted at dir
	srcRoot, err := fs.Sub(fsys, dir)
	if err != nil {
		return err
	}

	// clear existing stage state, build new stage state using
	// stage.digestAlgorithm.
	s.Logical = digest.NewMap()
	// checksum.WalkFunc
	each := func(j checksum.Job, err error) error {
		if err != nil {
			return err
		}
		sum, err := j.SumString(s.digestAlgorithm.ID())
		if err != nil {
			return err
		}
		err = s.Logical.Add(sum, j.Path())
		if err != nil {
			return err
		}
		return s.ProvidePath(srcRoot, j.Path(), sum, j.Path())
	}
	// FIXME: checksum.Walk is a mess.
	err = checksum.Walk(ocfl.NewFS(srcRoot), ".", each,
		checksum.WithAlg(s.digestAlgorithm.ID(), s.digestAlgorithm.New))
	if err != nil {
		s.err = err
		return err
	}
	// err = stage.writeInventory()
	// if err != nil {
	// 	stage.err = err
	// 	return err
	// }
	// return err
	return nil
}

// func (stage *Stage) SetState(state *digest.Map) error {
// 	if stage.err != nil {
// 		return ErrDirtyState
// 	}
// 	stage.State = state
// 	return nil
// }

// ProvidePath writes the file name in fsys to the stage version's content
// directory, saving it as path content. Path content is added to the stage
// manifest. If the digest is not part of the stage state an error is returned.
func (s *objStage) ProvidePath(fsys fs.FS, name, sum, content string) error {
	if s.err != nil {
		return errDirtyState
	}
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return s.ProvideReader(f, sum, content)
}

func (s *objStage) ProvideReader(src io.Reader, sum, content string) error {
	if s.err != nil {
		return errDirtyState
	}
	if s.Logical == nil {
		s.Logical = digest.NewMap()
	}
	if !s.Logical.DigestExists(sum) {
		return fmt.Errorf("digest not part of stage state: %s", sum)
	}
	if s.PrevContent != nil && s.PrevContent.DigestExists(sum) {
		log.Printf("duplicate (skipping): %s", sum)
		return nil
	}
	if s.NewContent == nil {
		s.NewContent = digest.NewMap()
	}
	if s.NewContent.DigestExists(sum) {
		log.Printf("duplicate (skipping): %s", sum)
		return nil
	}
	err := s.NewContent.Add(sum, content)
	if err != nil {
		s.err = err
		return err
	}
	dst := path.Join(s.stageRoot, contentDir, content)
	_, err = s.fsys.Write(context.TODO(), dst, src)
	if err != nil {
		s.err = err
		return s.err
	}
	log.Printf("staged: %s", sum)
	return nil
}

// missing content returns slice of digests for content that
// has not been provided to the stage
func (s *objStage) MissingContent() []string {
	var missing []string
	if s.Logical == nil {
		return nil
	}
	for d := range s.Logical.AllDigests() {
		if !s.NewContent.DigestExists(d) {
			missing = append(missing, d)
		}
	}
	return missing
}

type ContentPathFunc func(string) string

// BuildManifest is used to remap content paths in the stage to
// content paths in the new version
func (s *objStage) BuildManifest(f ContentPathFunc) (*digest.Map, error) {
	man := digest.NewMap()
	// copy existing
	if s.PrevContent != nil {
		for p, d := range s.PrevContent.AllPaths() {
			if err := man.Add(d, p); err != nil {
				return nil, err
			}
		}
	}
	if s.NewContent != nil {
		for p, d := range s.NewContent.AllPaths() {
			if err := man.Add(d, f(p)); err != nil {
				return nil, err
			}
		}
	}
	return man, nil
}

// func (s *objStage) Clear() error {
// 	s.err = errors.New("stage was cleared")
// 	if s.stageRoot == "" {
// 		return nil
// 	}
// 	return s.fsys.RemoveAll(s.stageRoot)
// }
