-- Write your migrate up statements here

{{ template "migrations/shared/trigger_set_timestamp.sql" . }}

CREATE TABLE feeds (
  publication_uuid uuid primary key,
  url text NOT NULL,
  language_code varchar(2) NOT NULL,
  last_modified timestamp,
  etag text,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  modified_at timestamptz NOT NULL DEFAULT NOW()
);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON "feeds" FOR EACH ROW EXECUTE PROCEDURE trigger_set_timestamp();

create table "processed_items" (
  guid text PRIMARY KEY,
  feeds_publication_uuid uuid NOT NULL REFERENCES feeds(publication_uuid) ON DELETE CASCADE,
  pubDate timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  modified_at timestamptz NOT NULL DEFAULT NOW()
);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON "processed_items" FOR EACH ROW EXECUTE PROCEDURE trigger_set_timestamp();

---- create above / drop below ----

DROP trigger set_timestamp ON "feeds";

DROP trigger set_timestamp ON "processed_items";

DROP FUNCTION trigger_set_timestamp;

DROP TABLE "processed_items";
DROP TABLE "feeds";
DROP TABLE "world_languages";

-- Write your migrate down statements here. If this migration is irreversible
-- Then delete the separator line above.