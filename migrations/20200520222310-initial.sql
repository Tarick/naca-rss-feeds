-- +migrate Up
CREATE TABLE feeds (
  publication_uuid uuid primary key,
  url text NOT NULL,
  last_modified timestamp,
  etag text,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  modified_at timestamptz NOT NULL DEFAULT NOW()
);

create table "processed_items" (
  guid text PRIMARY KEY,
  feeds_publication_uuid uuid NOT NULL REFERENCES feeds(publication_uuid) ON DELETE CASCADE,
  pubDate timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  modified_at timestamptz NOT NULL DEFAULT NOW()
);

-- +migrate StatementBegin
CREATE
OR REPLACE FUNCTION trigger_set_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.modified_at = NOW();

RETURN NEW;

END;

$$ LANGUAGE plpgsql;

-- +migrate StatementEnd
CREATE TRIGGER set_timestamp BEFORE
UPDATE
  ON "feeds" FOR EACH ROW EXECUTE PROCEDURE trigger_set_timestamp();

CREATE TRIGGER set_timestamp BEFORE
UPDATE
  ON "processed_items" FOR EACH ROW EXECUTE PROCEDURE trigger_set_timestamp();

-- +migrate Down
DROP trigger set_timestamp ON "feeds";

DROP trigger set_timestamp ON "processed_items";

DROP FUNCTION trigger_set_timestamp;

DROP TABLE "processed_items";
DROP TABLE "feeds";