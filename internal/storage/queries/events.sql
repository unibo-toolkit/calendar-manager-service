-- name: GetEventsForCalendar :many
SELECT
    te.id AS timetable_event_id,
    te.title,
    te.start_datetime,
    te.end_datetime,
    te.professor,
    te.module_code,
    te.credits,
    te.is_remote,
    te.teams_link,
    te.notes,
    te.group_id,
    0::integer AS sequence,
    c.name AS classroom_name,
    c.address AS classroom_address,
    c.latitude,
    c.longitude
FROM timetable_events te
LEFT JOIN classrooms c ON c.id = te.classroom_id
WHERE te.subject_id IN (
    SELECT cs.subject_id
    FROM calendar_subjects cs
    JOIN calendar_courses cc ON cc.id = cs.calendar_course_id
    WHERE cc.calendar_id = $1
)
ORDER BY te.start_datetime;

-- name: CountEventsForCalendar :one
SELECT COUNT(*)::integer AS total
FROM timetable_events te
WHERE te.subject_id IN (
    SELECT cs.subject_id
    FROM calendar_subjects cs
    JOIN calendar_courses cc ON cc.id = cs.calendar_course_id
    WHERE cc.calendar_id = $1
);