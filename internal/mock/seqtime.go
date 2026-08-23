package mock

import "time"

// seqEpoch anchors the fake instants the in-memory stores hand out.
//
// Several mocks used a row counter as a timestamp, which stopped type-checking
// once a stored instant became a time.Time. A counter of seconds from a fixed
// point keeps what those stores actually needed — a strictly increasing,
// reproducible order — without pretending to be the wall clock.
var seqEpoch = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

func seqTime(n int) time.Time { return seqEpoch.Add(time.Duration(n) * time.Second) }
