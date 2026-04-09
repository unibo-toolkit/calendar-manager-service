-- name: CreateCalendarLink :one
INSERT INTO calendar_links (slug, owner_id, name, description, lang, ttl_expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCalendarByID :one
SELECT * FROM calendar_links WHERE id = $1;

-- name: GetCalendarBySlug :one
SELECT * FROM calendar_links WHERE slug = $1;

-- name: ListCalendarsByOwner :many
SELECT
    cl.*,
    (SELECT COUNT(*) FROM calendar_courses WHERE calendar_id = cl.id) AS courses_count
FROM calendar_links cl
WHERE cl.owner_id = $1
ORDER BY cl.created_at DESC;

-- name: UpdateCalendarFields :one
UPDATE calendar_links
SET name = COALESCE(NULLIF(sqlc.narg('name')::text, ''), name),
    description = COALESCE(sqlc.narg('description')::text, description),
    lang = COALESCE(NULLIF(sqlc.narg('lang')::text, ''), lang),
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: DeleteCalendar :exec
DELETE FROM calendar_links WHERE id = $1;

-- name: ClaimCalendar :one
UPDATE calendar_links
SET owner_id = $2, ttl_expires_at = $3, updated_at = NOW()
WHERE id = $1 AND owner_id IS NULL
RETURNING *;

-- name: ExtendTTL :exec
UPDATE calendar_links
SET ttl_expires_at = $2
WHERE id = $1;

-- name: IncrementAccess :exec
UPDATE calendar_links
SET access_count = access_count + 1, last_accessed_at = NOW()
WHERE id = $1;

-- name: CreateCalendarCourse :one
INSERT INTO calendar_courses (calendar_id, curriculum_id, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteCalendarCoursesByCalendar :exec
DELETE FROM calendar_courses WHERE calendar_id = $1;

-- name: CreateCalendarSubject :exec
INSERT INTO calendar_subjects (calendar_course_id, subject_id)
VALUES ($1, $2);

-- name: GetCalendarCoursesWithDetails :many
SELECT
    cc.id AS calendar_course_id,
    cc.curriculum_id,
    cc.position,
    cur.code AS curriculum_code,
    cur.academic_year,
    cur.label AS curriculum_label,
    c.id AS course_id,
    c.title_it AS course_title_it,
    c.title_en AS course_title_en
FROM calendar_courses cc
JOIN curricula cur ON cur.id = cc.curriculum_id
JOIN courses c ON c.id = cur.course_id
WHERE cc.calendar_id = $1
ORDER BY cc.position;

-- name: GetCalendarSubjects :many
SELECT
    cs.calendar_course_id,
    s.id AS subject_id,
    s.title,
    s.module_code,
    s.credits,
    s.professor
FROM calendar_subjects cs
JOIN subjects s ON s.id = cs.subject_id
WHERE cs.calendar_course_id = ANY($1::uuid[])
ORDER BY s.title;

-- name: CheckCurriculumExists :one
SELECT id FROM curricula WHERE id = $1 AND is_active = true;

-- name: CheckSubjectBelongsToCurriculum :one
SELECT id FROM subjects WHERE id = $1 AND curriculum_id = $2 AND is_active = true;

-- name: GetActiveCalendarsCount :one
SELECT COUNT(*)::integer AS total FROM calendar_links WHERE ttl_expires_at > NOW();
