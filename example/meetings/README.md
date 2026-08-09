# Meetings / Calendar Example Data

A meeting schedule, for conflict detection, per-person schedules, and room
utilization.

## Files

| File | Contents |
|---|---|
| `meetings.csv` | Start, end, room, attendees, title, duration in minutes |

## Columns

- **start_time** — start timestamp
- **end_time** — end timestamp
- **room** — `会议室A` (room A) or `会议室B` (room B)
- **attendees** — semicolon-separated names
- **title** — meeting subject
- **duration_min** — duration in minutes

## Query ideas

- Every meeting one person attends — `grep` the `attendees` column
- Whether a room is occupied during a window, by overlapping intervals
- Filter by date or room
- Total meeting time, per person or per room
