package ocfl_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestValidateInventoryBytes(t *testing.T) {
	type testCase struct {
		inventory string
		expect    func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation)
	}
	var testInventories = map[string]testCase{
		// Good inventories
		`minimal`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.1/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				be.Equal(t, "http://example.org/minimal_no_content", inv.ID)
				be.Equal(t, "sha512", inv.DigestAlgorithm)
				be.Equal(t, "v1", inv.Head.String())
				be.Equal(t, ocfl.Spec1_1.InventoryType(), inv.Type)
				version := inv.Versions[inv.Head]
				be.Nonzero(t, version.Created)
				be.Equal(t, "One version and no content", version.Message)
				be.Equal(t, "mailto:Person_A@example.org", version.User.Address)
				be.Equal(t, "Person A", version.User.Name)
				// be.Nonzero(t, inv.Digest())
			},
		},
		`complete`: {
			inventory: `{
			"contentDirectory": "custom",
			"digestAlgorithm": "sha512",
			"fixity": {
				"md5": {
					"e8f239a71aabe2231faf696d92c92c20": [ "v1/content/file.txt" ]
				},
				"sha1": {
					"43c8321bda03dea62b63a5c09e9105b24ab6121b": [ "v1/content/file.txt" ]
				},
				"sha256": {
					"0b13a01dc7580ed7d4737d62ecd1a0c2067b0f3eccc327f4964fd82d582e3fd4": [ "v1/content/file.txt" ]
				},
				"sha512": {
					"a8a450d00c6ca7aa90e3e4858864fc195b6b2fe0a75c2d1e078e92eca232ce7be034a129ea9ea9cda2b0efaf11ba8f5ebdbebacb12f7992a4c37cad589e16a4d": [ "v1/content/file.txt" ]
				},
				"blake2b-512": {
					"51ff3faaf6b51b56011aea528fde0c43af07912011d1baa4fba795b899aa96e01452afc32d757777695bb9c93add6e8cb166b5e6f1c3670d9950e15570922203": [ "v1/content/file.txt" ]
				}
			},
			"head": "v1",
			"id": "info:something/abc",
			"manifest": {
				"a8a450d00c6ca7aa90e3e4858864fc195b6b2fe0a75c2d1e078e92eca232ce7be034a129ea9ea9cda2b0efaf11ba8f5ebdbebacb12f7992a4c37cad589e16a4d": ["v1/content/file.txt"]
			},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
				"v1": {
					"created": "2000-01-02T03:04:05Z",
					"state": {
						"a8a450d00c6ca7aa90e3e4858864fc195b6b2fe0a75c2d1e078e92eca232ce7be034a129ea9ea9cda2b0efaf11ba8f5ebdbebacb12f7992a4c37cad589e16a4d": ["file.txt"]
					},
					"message": "A file",
					"user": {
						"name": "A Person",
						"address": "https://orcid.org/0000-0000-0000-0000"
					}
				}
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				sum := "a8a450d00c6ca7aa90e3e4858864fc195b6b2fe0a75c2d1e078e92eca232ce7be034a129ea9ea9cda2b0efaf11ba8f5ebdbebacb12f7992a4c37cad589e16a4d"
				be.Equal(t, "custom", inv.ContentDirectory)
				be.Equal(t, "e8f239a71aabe2231faf696d92c92c20", inv.GetFixity(sum)["md5"])
				be.Equal(t, "v1/content/file.txt", inv.Manifest[sum][0])
				be.Equal(t, "file.txt", inv.Versions[inv.Head].State[sum][0])
			},
		},
		`one_version`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "ark:123/abc",
			"manifest": {
			  "43a43fe8a8a082d3b5343dfaf2fd0c8b8e370675b1f376e92e9994612c33ea255b11298269d72f797399ebb94edeefe53df243643676548f584fb8603ca53a0f": [
				"v1/content/a_file.txt"
			  ]
			},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "An version with one file",
				"state": {
				  "43a43fe8a8a082d3b5343dfaf2fd0c8b8e370675b1f376e92e9994612c33ea255b11298269d72f797399ebb94edeefe53df243643676548f584fb8603ca53a0f": [
					"a_file.txt"
				  ]
				},
				"user": {
				  "address": "mailto:a_person@example.org",
				  "name": "A Person"
				}
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
			},
		},
		// Warn inventories
		`missing_version_user`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				testutil.ErrorsIncludeOCFLCode(t, "W007", v.WarnErrors()...)
			},
		},
		// Warn inventories
		`missing_version_message`: {
			inventory: `{
				"digestAlgorithm": "sha512",
				"head": "v1",
				"id": "http://example.org/minimal_no_content",
				"manifest": {},
				"type": "https://ocfl.io/1.0/spec/#inventory",
				"versions": {
				  "v1": {
					"created": "2019-01-01T02:03:04Z",
					"state": { },
					"user": {
				  		"address": "mailto:a_person@example.org",
				  		"name": "A Person"
					}
				  }
				}
			  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				testutil.ErrorsIncludeOCFLCode(t, "W007", v.WarnErrors()...)
			},
		},
		// Bad inventories
		`missing_id`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E036", v.Errors()...)
			},
		},
		`bad_digestAlgorithm`: {
			inventory: `{
			"digestAlgorithm": "sha51",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E025", v.Errors()...)
			},
		},
		`missing_digestAlgorithm`: {
			inventory: `{
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E036", v.Errors()...)
			},
		},
		`null_id`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": null,
			"head": "v1",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E036", v.Errors()...)
			},
		},
		`missing_type`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"manifest": {},
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E036", v.Errors()...)
			},
		},
		`bad_type`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io",
			"manifest": {},
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E038", v.Errors()...)
			},
		},
		`bad_contentDirectory`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"contentDirectory": "..",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {},
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E017", v.Errors()...)
			},
		},
		`missing_head`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E040", v.Errors()...)
			},
		},
		`bad_head_format`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "1",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E040", v.Errors()...)
			},
		},
		`bad_head_not_last`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "version 1",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  },
			  "v2": {
				"created": "2019-02-01T02:03:04Z",
				"message": "version 1",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E040", v.Errors()...)
			},
		},
		`missing_manifest`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E041", v.Errors()...)
			},
		},
		`bad_manifest`: {

			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": 12,
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E041", v.Errors()...)
			},
		},
		`missing_versions`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E043", v.Errors()...)
			},
		},
		`bad_versions_empty`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {},
			"versions": {}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E008", v.Errors()...)
			},
		},
		`bad_versions_missingv1`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v2",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {},
			"versions": {
				"v2": {
					"created": "2019-01-01T02:03:04Z",
					"message": "One version and no content",
					"state": { },
					"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
				}
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E010", v.Errors()...)
			},
		},
		`bad_versions_padding`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v02",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {},
			"versions": {
				"v1": {
					"created": "2019-01-01T02:03:04Z",
					"message": "One version and no content",
					"state": { },
					"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
				},
				"v02": {
					"created": "2019-01-01T02:03:04Z",
					"message": "One version and no content",
					"state": { },
					"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
				}
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E012", v.Errors()...)
			},
		},
		`bad_manifest_digestconflict`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v2",
			"id": "uri:something451",
			"manifest": {
			  "10c4f059fc9235474c75c5e4b48837d1fcd93f6bca273c1153deb568096e1ec18fe5cd13467e550ca9dcfe8d4f81b2f71d5951a169cbfb321445a9a3211be708": [
				"v2/content/a_file.txt"
			  ],
			  "10C4F059FC9235474C75C5E4B48837D1FCD93F6BCA273C1153DEB568096E1EC18FE5CD13467E550CA9DCFE8D4F81B2F71D5951A169CBFB321445A9A3211BE708": [
				"v1/content/a_file.txt"
			  ]
			},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T01:01:01Z",
				"state": {
				  "10C4F059FC9235474C75C5E4B48837D1FCD93F6BCA273C1153DEB568096E1EC18FE5CD13467E550CA9DCFE8D4F81B2F71D5951A169CBFB321445A9A3211BE708": [
					"a_file.txt"
				  ]
				},
				"message": "Store version 1",
				"user": {
				  "name": "Sombody",
				  "address": "https://orcid.org/0000-0000-0000-0000"
				}
			  },
			  "v2": {
				"created": "2019-01-01T02:02:02Z",
				"state": {
				  "10c4f059fc9235474c75c5e4b48837d1fcd93f6bca273c1153deb568096e1ec18fe5cd13467e550ca9dcfe8d4f81b2f71d5951a169cbfb321445a9a3211be708": [
					"a_file.txt"
				  ]
				},
				"message": "Store version 2",
				"user": {
				  "name": "Sombody",
				  "address": "https://orcid.org/0000-0000-0000-0000"
				}
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E096", v.Errors()...)
			},
		},
		`bad_manifest_basepathconflict`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v3",
			"id": "uri:something451",
			"manifest": {
			  "10c4f059fc9235474c75c5e4b48837d1fcd93f6bca273c1153deb568096e1ec18fe5cd13467e550ca9dcfe8d4f81b2f71d5951a169cbfb321445a9a3211be708": [
				"v1/content/a_file/name.txt"
			  ],
			  "43a43fe8a8a082d3b5343dfaf2fd0c8b8e370675b1f376e92e9994612c33ea255b11298269d72f797399ebb94edeefe53df243643676548f584fb8603ca53a0f": [
				"v1/content/a_file"
			  ],
			  "8ed2115b36fe2d4db1b5ddad63f0deb13db339d3ff17f69fafb8cc8e9a20b89add82933d544b5512350a7f85cfae7e7235409c364060653e39ef9b18a81976fb": [
				"v2/content/a_file.txt"
			  ]
			},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T01:01:01Z",
				"state": {
				  "43a43fe8a8a082d3b5343dfaf2fd0c8b8e370675b1f376e92e9994612c33ea255b11298269d72f797399ebb94edeefe53df243643676548f584fb8603ca53a0f": [
					"a_file.txt"
				  ]
				},
				"message": "Store version 1",
				"user": {
				  "name": "Sombody",
				  "address": "https://orcid.org/0000-0000-0000-0000"
				}
			  },
			  "v2": {
				"created": "2019-01-01T02:02:02Z",
				"state": {
				  "10c4f059fc9235474c75c5e4b48837d1fcd93f6bca273c1153deb568096e1ec18fe5cd13467e550ca9dcfe8d4f81b2f71d5951a169cbfb321445a9a3211be708": [
					"a_file.txt"
				  ]
				},
				"message": "Store version 2",
				"user": {
				  "name": "Sombody",
				  "address": "https://orcid.org/0000-0000-0000-0000"
				}
			  },
			  "v3": {
				"created": "2019-01-01T03:03:03Z",
				"state": {
				  "8ed2115b36fe2d4db1b5ddad63f0deb13db339d3ff17f69fafb8cc8e9a20b89add82933d544b5512350a7f85cfae7e7235409c364060653e39ef9b18a81976fb": [
					"a_file.txt"
				  ]
				},
				"message": "Store version 1",
				"user": {
				  "name": "Sombody",
				  "address": "https://orcid.org/0000-0000-0000-0000"
				}
			  }
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E101", v.Errors()...)
			},
		},
		`missing_version_state`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E048", v.Errors()...)
			},
		},
		`null_version_block`: {
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
			},
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": null
			}
		  }`,
		},
		`missing_version_created`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"state": {},
				"message": "One version and no content",
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E048", v.Errors()...)
			},
		},
		`invalid_version_created`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
			  	"created": "2019",
				"state": {},
				"message": "One version and no content",
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E049", v.Errors()...)
			},
		},
		`missing_version_user_name`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"state": {},
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"user": { "address": "mailto:Person_A@example.org"}
			  }
			}
		}`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E054", v.Errors()...)
			},
		},
		`empty_version_user_name`: {
			inventory: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"versions": {
			  "v1": {
				"state": {},
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"user": { "address": "mailto:Person_A@example.org", "name": ""}
			  }
			}
		  }`,
			expect: func(t *testing.T, inv *ocfl.StoredInventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E054", v.Errors()...)
			},
		},
	}
	for desc, test := range testInventories {
		t.Run(desc, func(t *testing.T) {
			inv, v := ocfl.ValidateInventoryBytes([]byte(test.inventory))
			test.expect(t, inv, v)
		})
	}
}

func TestInventoryBuilder(t *testing.T) {
	t.Run("without previous", func(t *testing.T) {
		t.Run("complete", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "abc", Address: "email"}
			state := ocfl.DigestMap{"abc": []string{"file.txt"}}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				AddVersion(state, digest.SHA256, now, "message", user).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, ocfl.Spec1_1, inv.Type.Spec)
			be.Equal(t, ocfl.V(1), inv.Head)
			be.Equal(t, digest.SHA256.ID(), inv.DigestAlgorithm)
			version := inv.Versions[inv.Head]
			be.Nonzero(t, version)
			be.Equal(t, "message", inv.Versions[inv.Head].Message)
			be.Equal(t, now, version.Created)
			be.Equal(t, *user, *version.User)
			be.True(t, state.Eq(version.State))
			be.True(t, inv.Manifest.Eq(ocfl.DigestMap{
				"abc": []string{"v1/content/file.txt"},
			}))
		})
		t.Run("custom padding", func(t *testing.T) {
			state := ocfl.DigestMap{}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				AddVersion(state, digest.SHA256, time.Time{}, "message", nil).
				Padding(2).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, 2, inv.Head.Padding())
		})

		t.Run("custom spec", func(t *testing.T) {
			state := ocfl.DigestMap{}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				Spec(ocfl.Spec1_0).
				AddVersion(state, digest.SHA256, time.Time{}, "message", nil).
				Padding(2).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, ocfl.Spec1_0, inv.Type.Spec)
		})

		t.Run("fixty source", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "abc", Address: "email"}
			state := ocfl.DigestMap{"abc": []string{"file.txt"}}
			source := fixtySource{
				"abc": digest.Set{"md5": "123"},
			}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				AddVersion(state, digest.SHA256, now, "message", user).
				FixitySource(source).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, "123", inv.GetFixity("abc")["md5"])
		})

		t.Run("content directory", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "abc", Address: "email"}
			state := ocfl.DigestMap{"abc": []string{"file.txt"}}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				AddVersion(state, digest.SHA256, now, "message", user).
				ContentDirectory("stuff").
				Finalize()
			be.NilErr(t, err)
			be.True(t, inv.Manifest.Eq(ocfl.DigestMap{
				"abc": []string{"v1/stuff/file.txt"},
			}))
		})

		t.Run("content path func", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "abc", Address: "email"}
			state := ocfl.DigestMap{"abc": []string{"file.txt"}}
			contentFunc := func(paths []string) []string {
				for i, val := range paths {
					paths[i] = strings.ToUpper(val)
				}
				return paths
			}
			inv, err := ocfl.NewInventoryBuilder(nil).
				ID("test").
				AddVersion(state, digest.SHA256, now, "message", user).
				ContentPathFunc(contentFunc).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, "v1/content/FILE.TXT", inv.Manifest["abc"][0])
		})

		t.Run("missing id", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "abc", Address: "email"}
			state := ocfl.DigestMap{"abc": []string{"file.txt"}}
			_, err := ocfl.NewInventoryBuilder(nil).
				AddVersion(state, digest.SHA256, now, "message", user).
				Finalize()
			be.Nonzero(t, err)
		})

		t.Run("missing versions", func(t *testing.T) {
			_, err := ocfl.NewInventoryBuilder(nil).
				ID("tests").
				Finalize()
			be.Nonzero(t, err)
		})
	})
	t.Run("with previous", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		user := &ocfl.User{Name: "name", Address: "email"}
		state := ocfl.DigestMap{"abc": []string{"file.txt"}}
		padding := 3
		fixty := fixtySource{"abc": digest.Set{"md5": "123"}}
		baseInv, err := ocfl.NewInventoryBuilder(nil).
			ID("test").
			Spec(ocfl.Spec1_0).
			ContentDirectory("files").
			FixitySource(fixty).
			Padding(padding).
			AddVersion(state, digest.SHA256, now, "init", user).
			Finalize()
		be.NilErr(t, err)
		t.Run("complete", func(t *testing.T) {
			now := time.Now().Truncate(time.Second)
			user := &ocfl.User{Name: "name2", Address: "email2"}
			v2State := ocfl.DigestMap{
				"def": []string{"file2.txt"},
			}
			fixty := fixtySource{
				"abc": digest.Set{"size": "1"},
				"def": digest.Set{"size": "2"},
			}
			inv, err := ocfl.NewInventoryBuilder(baseInv).
				AddVersion(v2State, digest.SHA256, now, "update", user).
				FixitySource(fixty).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, ocfl.Spec1_0, inv.Type.Spec)
			be.Equal(t, ocfl.V(2, padding), inv.Head)
			be.Equal(t, digest.SHA256.ID(), inv.DigestAlgorithm)
			ver := inv.Versions[inv.Head]
			be.Equal(t, "update", ver.Message)
			be.Equal(t, now, ver.Created)
			be.Equal(t, *user, *ver.User)
			be.True(t, v2State.Eq(ver.State))
			// original and new manifest values are present
			be.True(t, inv.Manifest.Eq(ocfl.DigestMap{
				"abc": []string{"v001/files/file.txt"},
				"def": []string{"v002/files/file2.txt"},
			}))
			// original and new fixity values are present
			be.Equal(t, "1", inv.GetFixity("abc")["size"])
			be.Equal(t, "2", inv.GetFixity("def")["size"])
			be.Equal(t, "123", inv.GetFixity("abc")["md5"])
		})
		t.Run("padding is ignored", func(t *testing.T) {
			inv, err := ocfl.NewInventoryBuilder(baseInv).
				AddVersion(ocfl.DigestMap{}, digest.SHA256, time.Time{}, "message", nil).
				Padding(2).
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, padding, inv.Head.Padding())
		})

		t.Run("content directory is ignored", func(t *testing.T) {
			inv, err := ocfl.NewInventoryBuilder(baseInv).
				AddVersion(ocfl.DigestMap{}, digest.SHA256, time.Time{}, "message", nil).
				ContentDirectory("content").
				Finalize()
			be.NilErr(t, err)
			be.Equal(t, "files", inv.ContentDirectory)
		})
	})

}

// fixity source used for testing
type fixtySource map[string]digest.Set

var _ ocfl.FixitySource = fixtySource(nil)

func (s fixtySource) GetFixity(dig string) digest.Set { return s[dig] }

func TestReadInventorySidecar(t *testing.T) {
	ctx := context.Background()
	goodObjectFixtures := filepath.Join(`testdata`, `object-fixtures`, `1.1`, `good-objects`)
	badObjectFixtures := filepath.Join(`testdata`, `object-fixtures`, `1.1`, `bad-objects`)

	t.Run("ok", func(t *testing.T) {
		expectDigest := "8e280eb94af68d27f635c2013531d4cf41c6089dfa8ffeeb4f0230500203fab9c10f929c08057f5d1b5084ab4dff7d72fb20010bf4cbf713569fadfc9257770a"
		digest, err := ocfl.ReadInventorySidecar(ctx, ocflfs.DirFS(goodObjectFixtures), "spec-ex-full", "sha512")
		be.NilErr(t, err)
		be.Equal(t, expectDigest, digest)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocfl.ReadInventorySidecar(ctx, ocflfs.DirFS(badObjectFixtures), "spec-ex-full", "sha256")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("invalid sidecar contents", func(t *testing.T) {
		_, err := ocfl.ReadInventorySidecar(ctx, ocflfs.DirFS(badObjectFixtures), "E061_invalid_sidecar", "sha512")
		be.Nonzero(t, err)
		be.True(t, errors.Is(err, ocfl.ErrInventorySidecarContents))
	})
}

func TestStoredInventory_Marshal(t *testing.T) {
	input := []byte(`
		{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"id": "http://example.org/minimal_no_content",
			"manifest": {},
			"type": "https://ocfl.io/1.1/spec/#inventory",
			"versions": {
				"v1": {
				"created": "2019-01-01T02:03:04Z",
				"message": "One version and no content",
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
				}
			}
		}
	`)
	digester := digest.SHA512.Digester()
	_, err := digester.Write(input)
	be.NilErr(t, err)
	var inv ocfl.StoredInventory
	be.NilErr(t, inv.UnmarshalBinary(input))
	be.Equal(t, digester.String(), inv.Digest())
	out, err := inv.MarshalBinary()
	be.NilErr(t, err)
	be.True(t, bytes.Equal(input, out))
}
