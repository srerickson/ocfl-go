package ocflv1_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

type testInventory struct {
	data   string
	expect func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation)
}

var testInventories = map[string]testInventory{
	// Good inventories
	`minimal`: {
		data: `{
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
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.NilErr(t, v.Err())
			be.Equal(t, "http://example.org/minimal_no_content", inv.ID)
			be.Equal(t, "sha512", inv.DigestAlgorithm)
			be.Equal(t, "v1", inv.Head.String())
			be.Equal(t, ocfl.Spec1_0.AsInvType(), inv.Type)
			version := inv.Versions[inv.Head]
			be.Nonzero(t, version.Created)
			be.Equal(t, "One version and no content", version.Message)
			be.Equal(t, "mailto:Person_A@example.org", version.User.Address)
			be.Equal(t, "Person A", version.User.Name)
		},
	},
	`minimal_contentDirectory`: {
		data: `{
			"digestAlgorithm": "sha512",
			"head": "v1",
			"contentDirectory": "cont",
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
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.NilErr(t, v.Err())
		},
	},
	`one_version`: {
		data: `{
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
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.NilErr(t, v.Err())
		},
	},
	// Bad inventories
	`missing_id`: {
		data: `{
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
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
	},
	`bad_digestAlgorithm`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`missing_digestAlgorithm`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`null_id`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`missing_type`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_type`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_contentDirectory`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"contentDirectory": "..",
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
	},
	`missing_head`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
			ErrorsIncludeOCFLCode(t, "E104", v.Errors()...)
		},
		data: `{
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
	},
	`bad_head_format`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
			ErrorsIncludeOCFLCode(t, "E104", v.Errors()...)
		},
		data: `{
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
	},
	`bad_head_not_last`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`missing_manifest`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_manifest`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`missing_versions`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {}
		  }`,
	},
	`bad_versions_empty`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
				"digestAlgorithm": "sha512",
				"id": "ark:123/abc",
				"head": "v1",
				"type": "https://ocfl.io/1.0/spec/#inventory",
				"manifest": {},
				"versions": {}
			  }`,
	},
	`bad_versions_missingv1`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_versions_padding`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_manifest_digestconflict`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`bad_manifest_basepathconflict`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
		  }
		  `,
	},
	`missing_version_state`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`null_version_block`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`missing_version_user_name`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
	`empty_version_user_name`: {
		expect: func(t *testing.T, inv *ocflv1.Inventory, v *ocfl.Validation) {
			be.True(t, v.Err() != nil)
		},
		data: `{
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
	},
}

func TestValidateInventory(t *testing.T) {
	for desc, test := range testInventories {
		t.Run(desc, func(t *testing.T) {
			inv, vldr := ocflv1.ValidateInventoryBytes([]byte(test.data), ocfl.Spec1_0)
			test.expect(t, inv, vldr)
		})
	}
}

func ErrorsIncludeOCFLCode(t *testing.T, ocflCode string, errs ...error) {
	t.Helper()
	var foundCodes []string
	for _, err := range errs {
		var vCode *ocfl.ValidationError
		if errors.As(err, &vCode) {
			foundCodes = append(foundCodes, vCode.Code)
		}
	}
	if !slices.Contains(foundCodes, ocflCode) {
		t.Errorf("OCFL validation code %q not in found validation codes %v", ocflCode, foundCodes)
	}
}
