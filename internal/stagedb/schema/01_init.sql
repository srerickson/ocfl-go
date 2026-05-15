-- schema version table
CREATE TABLE schema_versions (
    version INTEGER PRIMARY KEY
);

CREATE TABLE raw_inventories (
    digest TEXT PRIMARY KEY,
    bytes BLOB REQUIRED NOT NULL
);