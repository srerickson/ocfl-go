package ocfl

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xeipuuv/gojsonschema"
)

func TestInventoryMarshalling(t *testing.T) {
	inv := &Inventory{}
	test := `{
    "digestAlgorithm": "sha512",
    "fixity": {
      "md5": {
        "184f84e28cbe75e050e9c25ea7f2e939": [ "v1/content/foo/bar.xml" ],
        "2673a7b11a70bc7ff960ad8127b4adeb": [ "v2/content/foo/bar.xml" ],
        "c289c8ccd4bab6e385f5afdd89b5bda2": [ "v1/content/image.tiff" ],
        "d41d8cd98f00b204e9800998ecf8427e": [ "v1/content/empty.txt" ]
      },
      "sha1": {
        "66709b068a2faead97113559db78ccd44712cbf2": [ "v1/content/foo/bar.xml" ],
        "a6357c99ecc5752931e133227581e914968f3b9c": [ "v2/content/foo/bar.xml" ],
        "b9c7ccc6154974288132b63c15db8d2750716b49": [ "v1/content/image.tiff" ],
        "da39a3ee5e6b4b0d3255bfef95601890afd80709": [ "v1/content/empty.txt" ]
      }
    },
    "head": "v3",
    "id": "ark:/12345/bcd987",
    "manifest": {
      "4d27c8...b53": [ "v2/content/foo/bar.xml" ],
      "7dcc35...c31": [ "v1/content/foo/bar.xml" ],
      "cf83e1...a3e": [ "v1/content/empty.txt" ],
      "ffccf6...62e": [ "v1/content/image.tiff" ]
    },
    "type": "https://ocfl.io/1.0/spec/#inventory",
    "versions": {
      "v1": {
        "created": "2018-01-01T01:01:01Z",
        "message": "Initial import",
        "state": {
          "7dcc35...c31": [ "foo/bar.xml" ],
          "cf83e1...a3e": [ "empty.txt" ],
          "ffccf6...62e": [ "image.tiff" ]
        },
        "user": {
          "address": "alice@example.com",
          "name": "Alice"
        }
      },
      "v2": {
        "created": "2018-02-02T02:02:02Z",
        "message": "Fix bar.xml, remove image.tiff, add empty2.txt",
        "state": {
          "4d27c8...b53": [ "foo/bar.xml" ],
          "cf83e1...a3e": [ "empty.txt", "empty2.txt" ]
        },
        "user": {
          "address": "bob@example.com",
          "name": "Bob"
        }
      },
      "v3": {
        "created": "2018-03-03T03:03:03Z",
        "message": "Reinstate image.tiff, delete empty.txt",
        "state": {
          "4d27c8...b53": [ "foo/bar.xml" ],
          "cf83e1...a3e": [ "empty2.txt" ],
          "ffccf6...62e": [ "image.tiff" ]
        },
        "user": {
          "address": "cecilia@example.com",
          "name": "Cecilia"
        }
      }
    }
  }`
	if err := json.Unmarshal([]byte(test), inv); err != nil {
		t.Error(err)
	}
	if inv.Versions[`v1`].User.Address != `alice@example.com` {
		t.Error(`Problem Unmarshalling Versions Element`)
	}

	// Validate Marshalled JSON
	_, schemaPath, _, _ := runtime.Caller(0)
	schemaPath = filepath.Dir(schemaPath)
	schemaPath = fmt.Sprintf("file://%s/inventory_schema.json", schemaPath)
	newInv, _ := json.Marshal(inv)
	schemaLoader := gojsonschema.NewReferenceLoader(schemaPath)
	documentLoader := gojsonschema.NewBytesLoader(newInv)
	if _, err := gojsonschema.Validate(schemaLoader, documentLoader); err != nil {
		t.Error(err)
	}
}

func TestInventoryValidate(t *testing.T) {
	var inv = Inventory{}
	var invJson []byte
	var err error
	objectRoot := `test/fixtures/1.0/objects/spec-ex-full`
	file, err := os.Open(objectRoot + `/inventory.json`)
	if err != nil {
		t.Error(err)
	}
	if invJson, err = ioutil.ReadAll(file); err != nil {
		t.Error(err)
	}
	if err = json.Unmarshal(invJson, &inv); err != nil {
		t.Error(err)
	}
	if err = inv.ValidateManifest(objectRoot); err != nil {
		t.Error(err)
	}
	if err = inv.ValidateFixity(objectRoot); err != nil {
		t.Error(err)
	}

}
