package ocfl_test

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestValidateInventoryBytes(t *testing.T) {
	type testCase struct {
		inventory string
		expect    func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation)
	}
	var testInventories = map[string]testCase{
		// Good inventories
		`minimal`: {
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
				"state": { },
				"user": { "address": "mailto:Person_A@example.org", "name": "Person A" }
			  }
			}
		  }`,
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				be.Equal(t, "http://example.org/minimal_no_content", inv.ID())
				be.Equal(t, "sha512", inv.DigestAlgorithm().ID())
				be.Equal(t, "v1", inv.Head().String())
				be.Equal(t, ocfl.Spec1_0, inv.Spec())
				version := inv.Version(inv.Head().Num())
				be.Nonzero(t, version.Created)
				be.Equal(t, "One version and no content", version.Message())
				be.Equal(t, "mailto:Person_A@example.org", version.User().Address)
				be.Equal(t, "Person A", version.User().Name)
				be.Nonzero(t, inv.Digest())
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
				be.NilErr(t, v.Err())
				sum := "a8a450d00c6ca7aa90e3e4858864fc195b6b2fe0a75c2d1e078e92eca232ce7be034a129ea9ea9cda2b0efaf11ba8f5ebdbebacb12f7992a4c37cad589e16a4d"
				be.Equal(t, "custom", inv.ContentDirectory())
				be.Equal(t, "e8f239a71aabe2231faf696d92c92c20", inv.GetFixity(sum)["md5"])
				be.Equal(t, "v1/content/file.txt", inv.Manifest()[sum][0])
				be.Equal(t, "file.txt", inv.Version(0).State()[sum][0])
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
				be.True(t, v.Err() != nil)
				testutil.ErrorsIncludeOCFLCode(t, "E048", v.Errors()...)
			},
		},
		`null_version_block`: {
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
			expect: func(t *testing.T, inv ocfl.Inventory, v *ocfl.Validation) {
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
