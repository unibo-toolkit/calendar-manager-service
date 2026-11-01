-- name: CountKnownFamilies :one
SELECT COUNT(*)::integer AS total
FROM unnest(@family_hashes::text[]) AS h(family_hash)
WHERE EXISTS (
    SELECT 1
    FROM calendar_excluded_families f
    WHERE f.calendar_id = @calendar_id AND f.family_hash = h.family_hash
) OR EXISTS (
    SELECT 1
    FROM timetable_events te
    JOIN calendar_subjects cs ON cs.subject_id = te.subject_id
    JOIN calendar_courses cc ON cc.id = cs.calendar_course_id
    WHERE cc.calendar_id = @calendar_id AND te.family_hash = h.family_hash
);

-- name: DeleteExcludedFamiliesNotIn :exec
DELETE FROM calendar_excluded_families
WHERE calendar_id = @calendar_id
  AND family_hash <> ALL(@family_hashes::text[]);

-- name: InsertExcludedFamilies :exec
INSERT INTO calendar_excluded_families (calendar_id, family_hash)
SELECT @calendar_id, unnest(@family_hashes::text[])
ON CONFLICT (calendar_id, family_hash) DO NOTHING;

-- name: ListExcludedFamilies :many
SELECT family_hash
FROM calendar_excluded_families
WHERE calendar_id = $1
ORDER BY created_at, family_hash;
