-- Arcs: a ticket may point at a parent ticket (an arc). No arcs table —
-- an arc IS a ticket (slug, gates, history all reused); it is any ticket
-- that other tickets reference via parent. Provenance fields (source,
-- source_ref), dependencies (depends) and area stay free jsonb fields;
-- only parent needs a column because arc rollups query it directly.

ALTER TABLE tickets ADD COLUMN parent text REFERENCES tickets(ulid);
CREATE INDEX tickets_parent_idx ON tickets (parent);
