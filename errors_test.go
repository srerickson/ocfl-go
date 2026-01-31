package ocfl

var _ error = &MapDigestConflictErr{}
var _ error = &MapPathConflictErr{}
var _ error = &MapPathInvalidErr{}
var _ error = ErrMapMakerExists
