package ocflv1_test

import (
	"context"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

func TestNewInventory(t *testing.T) {
	ctx := context.Background()
	base, validation := ocflv1.ValidateInventoryReader(ctx, strings.NewReader(testInv))
	if err := validation.Err(); err != nil {
		t.Fatal("test inventory isn't valid:", err)
	}
	// new version state
	version := &ocflv1.Version{State: ocfl.DigestMap{
		"abc": []string{"newfile.txt"},
	}}
	// fixity values that should be added to new inventory
	fixity := fixitySource{
		// md5 for existing content: v1/content/foo/bar.xml
		"7dcc352f96c56dc5b094b2492c2866afeb12136a78f0143431ae247d02f02497bbd733e0536d34ec9703eba14c6017ea9f5738322c1d43169f8c77785947ac31": ocfl.DigestSet{
			"md5": "184f84e28cbe75e050e9c25ea7f2e939",
		},
		// fake digest for new content
		"abc": {
			"sha256": "def",
			"md5":    "ghi",
			"sha1":   "jkl",
		},
	}
	result, err := ocflv1.NewInventory(base, version, fixity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fixity[ocfl.SHA256]) < 1 {
		t.Fatal("missing fixity block")
	}
	if len(result.Manifest) != len(base.Manifest)+1 {
		t.Fatal("new inventory should have one additional manifest entry")
	}
	if len(result.Manifest["abc"]) < 1 {
		t.Fatal("expected manifest entry for new file")
	}
	for digest := range result.Manifest {
		set := result.GetFixity(digest)
		if set[ocfl.MD5] == "" {
			t.Fatal("missing md5")
		}
		if set[ocfl.SHA1] == "" {
			t.Fatal("missing sha1")
		}
	}
}

type fixitySource map[string]ocfl.DigestSet

func (f fixitySource) GetFixity(digest string) ocfl.DigestSet {
	return f[digest]
}

var testInv = `{
  "digestAlgorithm": "sha512",
  "fixity": {
    "md5": {
      "2673a7b11a70bc7ff960ad8127b4adeb": [
        "v2/content/foo/bar.xml"
      ],
      "c289c8ccd4bab6e385f5afdd89b5bda2": [
        "v1/content/image.tiff"
      ],
      "d41d8cd98f00b204e9800998ecf8427e": [
        "v1/content/empty.txt"
      ]
    },
    "sha1": {
      "66709b068a2faead97113559db78ccd44712cbf2": [
        "v1/content/foo/bar.xml"
      ],
      "a6357c99ecc5752931e133227581e914968f3b9c": [
        "v2/content/foo/bar.xml"
      ],
      "b9c7ccc6154974288132b63c15db8d2750716b49": [
        "v1/content/image.tiff"
      ],
      "da39a3ee5e6b4b0d3255bfef95601890afd80709": [
        "v1/content/empty.txt"
      ]
    }
  },
  "head": "v3",
  "id": "ark:/12345/bcd987",
  "manifest": {
    "4d27c86b026ff709b02b05d126cfef7ec3aed5f83f5e98df7d7592f7a44bd1dc7f29509cff06b884158baa36a2bbeda11ab8a64b56585a70f5ce1fa96e26eb53": [
      "v2/content/foo/bar.xml"
    ],
    "7dcc352f96c56dc5b094b2492c2866afeb12136a78f0143431ae247d02f02497bbd733e0536d34ec9703eba14c6017ea9f5738322c1d43169f8c77785947ac31": [
      "v1/content/foo/bar.xml"
    ],
    "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e": [
      "v1/content/empty.txt"
    ],
    "ffccf6baa21809716f31563fafb9f333c09c336bb7400088f17e4ff307f98fc9b14a577f92f3285913b7f53a6d5cf004503cf839aada1c885ac69336cbfb862e": [
      "v1/content/image.tiff"
    ]
  },
  "type": "https://ocfl.io/1.0/spec/#inventory",
  "versions": {
    "v1": {
      "created": "2018-01-01T01:01:01Z",
      "message": "Initial import",
      "state": {
        "7dcc352f96c56dc5b094b2492c2866afeb12136a78f0143431ae247d02f02497bbd733e0536d34ec9703eba14c6017ea9f5738322c1d43169f8c77785947ac31": [
          "foo/bar.xml"
        ],
        "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e": [
          "empty.txt"
        ],
        "ffccf6baa21809716f31563fafb9f333c09c336bb7400088f17e4ff307f98fc9b14a577f92f3285913b7f53a6d5cf004503cf839aada1c885ac69336cbfb862e": [
          "image.tiff"
        ]
      },
      "user": {
        "address": "mailto:alice@example.com",
        "name": "Alice"
      }
    },
    "v2": {
      "created": "2018-02-02T02:02:02Z",
      "message": "Fix bar.xml, remove image.tiff, add empty2.txt",
      "state": {
        "4d27c86b026ff709b02b05d126cfef7ec3aed5f83f5e98df7d7592f7a44bd1dc7f29509cff06b884158baa36a2bbeda11ab8a64b56585a70f5ce1fa96e26eb53": [
          "foo/bar.xml"
        ],
        "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e": [
          "empty.txt",
          "empty2.txt"
        ]
      },
      "user": {
        "address": "mailto:bob@example.com",
        "name": "Bob"
      }
    },
    "v3": {
      "created": "2018-03-03T03:03:03Z",
      "message": "Reinstate image.tiff, delete empty.txt",
      "state": {
        "4d27c86b026ff709b02b05d126cfef7ec3aed5f83f5e98df7d7592f7a44bd1dc7f29509cff06b884158baa36a2bbeda11ab8a64b56585a70f5ce1fa96e26eb53": [
          "foo/bar.xml"
        ],
        "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e": [
          "empty2.txt"
        ],
        "ffccf6baa21809716f31563fafb9f333c09c336bb7400088f17e4ff307f98fc9b14a577f92f3285913b7f53a6d5cf004503cf839aada1c885ac69336cbfb862e": [
          "image.tiff"
        ]
      },
      "user": {
        "address": "mailto:cecilia@example.com",
        "name": "Cecilia"
      }
    }
  }
}`
