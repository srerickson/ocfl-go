package ocflv1_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl-go/ocflv1"
	"github.com/srerickson/ocfl-go/validation"
)

type testInventory struct {
	valid       bool
	description string
	data        string
}

var testInventories = []testInventory{
	// Good inventories
	{
		valid:       true,
		description: `minimal`,
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
	},
	{
		valid:       true,
		description: `minimal_contentDirectory`,
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
	},
	{
		valid:       true,
		description: `one_version`,
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
	},
	// Bad inventories
	{
		valid:       false,
		description: `missing_id`,
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
	}, {
		valid:       false,
		description: `bad_digestAlgorithm`,
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
	}, {
		valid:       false,
		description: `missing_digestAlgorithm`,
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
	}, {
		valid:       false,
		description: `null_id`,
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
	{
		valid:       false,
		description: `missing_type`,
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
	{
		valid:       false,
		description: `bad_type`,
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
	{
		valid:       false,
		description: `bad_contentDirectory`,
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
	{
		valid:       false,
		description: `missing_head`,
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
	{
		valid:       false,
		description: `bad_head_format`,
		data: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1.0",
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
	{
		valid:       false,
		description: `bad_head_not_last`,
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
	{
		valid:       false,
		description: `missing_manifest`,
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
	{
		valid:       false,
		description: `bad_manifest`,
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
	{
		valid:       false,
		description: `missing_versions`,
		data: `{
			"digestAlgorithm": "sha512",
			"id": "ark:123/abc",
			"head": "v1",
			"type": "https://ocfl.io/1.0/spec/#inventory",
			"manifest": {}
		  }`,
	},
	{
		valid:       false,
		description: `bad_versions_empty`,
		data: `{
				"digestAlgorithm": "sha512",
				"id": "ark:123/abc",
				"head": "v1",
				"type": "https://ocfl.io/1.0/spec/#inventory",
				"manifest": {},
				"versions": {}
			  }`,
	},
	{
		valid:       false,
		description: `bad_versions_missingv1`,
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
	{
		valid:       false,
		description: `bad_versions_padding`,
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
	{
		valid:       false,
		description: `bad_manifest_digestconflict`,
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
	{
		valid:       false,
		description: `bad_manifest_basepathconflict`,
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
	{
		valid:       false,
		description: `missing_version_state`,
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
	{
		valid:       false,
		description: `null_version_block`,
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
	{
		valid:       false,
		description: `missing_version_created`,
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
	{
		valid:       false,
		description: `missing_version_user_name`,
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
	{
		valid:       false,
		description: `empty_version_user_name`,
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
	ctx := context.Background()
	for _, test := range testInventories {
		t.Run(test.description, func(t *testing.T) {
			is := is.New(t)
			reader := strings.NewReader(test.data)
			_, result := ocflv1.ValidateInventoryReader(ctx, reader)
			if test.valid {
				is.NoErr(result.Err())
			} else {
				err := result.Err()
				is.True(err != nil)
				if err != nil {
					var eCode validation.ErrorCode
					if !errors.As(err, &eCode) {
						t.Errorf(`err is not an ErrorCode: %v`, err)
					}
					var decodeErr *ocflv1.InvDecodeError
					if errors.As(err, &decodeErr) {
						if decodeErr.Field == "" {
							t.Errorf(`decode error has not Field value: %v`, err)
						}
					}
				}
			}
		})
	}
}

func FuzzValidateInventory(f *testing.F) {
	ctx := context.Background()
	for _, test := range testInventories {
		f.Add([]byte(test.data))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		reader := bytes.NewReader(b)
		_, result := ocflv1.ValidateInventoryReader(ctx, reader)
		err := result.Err()
		if err != nil {
			var eCode validation.ErrorCode
			if !errors.As(err, &eCode) {
				t.Errorf(`err is not an ErrorCode: %v`, err)
			}
			var decodeErr *ocflv1.InvDecodeError
			if errors.As(err, &decodeErr) {
				if decodeErr.Field == "" {
					t.Errorf(`decode error has not Field value: %v`, err)
				}
			}
		}
	})
}
