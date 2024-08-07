package ocflv1

import (
	"io/fs"
	"strings"
)

type versionDirState struct {
	hasInventory bool
	sidecarAlg   string
	extraFiles   []string
	dirs         []string
}

func parseVersionDirState(entries []fs.DirEntry) versionDirState {
	var info versionDirState
	for _, e := range entries {
		if e.Type().IsDir() {
			info.dirs = append(info.dirs, e.Name())
			continue
		}
		if e.Type().IsRegular() || e.Type() == fs.ModeIrregular {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if strings.HasPrefix(e.Name(), inventoryFile+".") && info.sidecarAlg == "" {
				info.sidecarAlg = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
		}
		// unexpected files
		info.extraFiles = append(info.extraFiles, e.Name())
	}
	return info
}

// validatePathLedger validates the pathLedger. Before running
// validatePathLedger, all inventories in the object should have been added to
// the ledger (with addInventory) and all version content should have been
// indexed (with indexVersionContent). The ledger is valid if (1) every path
// exists, (2) every path exists in the root inventory manifest, (3) every path
// exists in version inventory manifests equal of greater than the version in
// which the path is stored, (4) all digests for all paths are confirmed.
// func (vldr *objectValidator) validatePathLedger(ctx context.Context) error {
// 	ocflV := vldr.root.State.Spec
// 	lgr := vldr.opts.Logger
// 	// check paths exist are in included in manifsts as necessary
// 	for p, pInfo := range vldr.ledger.paths {
// 		pVer := pInfo.existsIn // version wheren content file is stored (or empty ocfl.Num)
// 		if pVer.IsZero() {
// 			for v, f := range pInfo.locations() {
// 				locStr := "root"
// 				if !f.InRoot() {
// 					locStr = v.String()
// 				}
// 				if f.InManifest() {
// 					err := fmt.Errorf("path referenced in %s inventory manifest does not exist: %s", locStr, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E092(ocflV)))
// 				}
// 				if f.InFixity() {
// 					err := fmt.Errorf("path referenced in %s inventory fixity does not exist: %s", locStr, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E093(ocflV)))
// 				}
// 			}
// 		}
// 		for v := range vldr.ledger.inventories {
// 			if v.Num() >= pVer.Num() {
// 				if !pInfo.referencedIn(v, inManifest) {
// 					err := fmt.Errorf("path not referenecd in %s manifest as expected: %s", v, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E023(ocflV)))
// 				}
// 			}
// 		}
// 	}
// 	// don't continue if there are previous errors
// 	if err := vldr.Err(); err != nil {
// 		return err
// 	}
// 	// don't continue if NoDigest is set
// 	if vldr.opts.SkipDigests {
// 		return nil
// 	}
// 	// digests
// 	var setupErr error
// 	digestSetup := func(digestFile func(name string, algs []string) bool) {
// 		for name, pInfo := range vldr.ledger.paths {
// 			algs := make([]string, 0, len(pInfo.digests))
// 			for alg := range pInfo.digests {
// 				algs = append(algs, alg)
// 			}
// 			if len(algs) == 0 {
// 				// no digests associate with the path
// 				err := fmt.Errorf("path not referenecd in manifest as expected: %s", name)
// 				setupErr = ec(err, codes.E023(ocflV))
// 				return
// 			}
// 			if !digestFile(path.Join(vldr.Root, name), algs) {
// 				setupErr = errors.New("digest validation interupted")
// 			}
// 		}
// 	}
// 	var digestErr error
// 	ocfl.Digest(ctx, vldr.FS, digestSetup)(func(r ocfl.DigestResult, err error) bool {
// 		if err != nil {
// 			digestErr = err
// 			if errors.Is(digestErr, fs.ErrNotExist) {
// 				digestErr = ec(digestErr, codes.E092(ocflV))
// 			}
// 			vldr.LogFatal(lgr, digestErr)
// 			return false
// 		}
// 		name := r.Path
// 		for alg, sum := range r.Digests {
// 			// convert path back from FS-relative to object-relative path
// 			objPath := strings.TrimPrefix(name, vldr.Root+"/")
// 			entry, exists := vldr.ledger.getDigest(objPath, alg)
// 			if !exists {
// 				panic(`BUG: path/algorithm not a valid key as expected`)
// 			}
// 			if !strings.EqualFold(sum, entry.digest) {
// 				digestErr = &ContentDigestErr{
// 					Path:   name,
// 					Alg:    alg,
// 					Entry:  *entry,
// 					Digest: sum,
// 				}
// 				for _, l := range entry.locs {
// 					if l.InManifest() {
// 						vldr.LogFatal(lgr, ec(digestErr, codes.E092(ocflV)))
// 					} else {
// 						vldr.LogFatal(lgr, ec(digestErr, codes.E093(ocflV)))
// 					}
// 				}
// 				return false
// 			}
// 		}
// 		return true
// 	})
// 	if err := errors.Join(setupErr, digestErr); err != nil {
// 		vldr.LogFatal(lgr, err)
// 		return err
// 	}
// 	return nil
// }
