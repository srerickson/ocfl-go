package schema

import _ "embed"

//go:embed inventory_schema.json
var InventorySchema string
var _ = InventorySchema
