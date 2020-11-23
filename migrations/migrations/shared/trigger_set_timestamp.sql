
CREATE OR REPLACE FUNCTION trigger_set_timestamp() RETURNS TRIGGER AS $$ BEGIN NEW.modified_at = NOW();

RETURN NEW;

END;

$$ LANGUAGE plpgsql;
