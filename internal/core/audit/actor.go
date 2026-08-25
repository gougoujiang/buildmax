package audit

// Actor is who did the thing an event records.
//
// The two kinds are different authorities rather than two names for one. An
// operator proved it by reaching the machine and its database credentials; a
// user proved it with a session and whatever grant the route required. A reader
// months later needs to tell an emergency shell change from a routine one, and
// a rule that differs by authority -- revoking the last administrator, say --
// needs something better to key on than a boolean any caller could set.
type Actor struct {
	Type string
	ID   string
}

// OperatorActor is the shell. The process cannot name the person at it; being
// able to run the command with the database credentials is the authorization.
func OperatorActor() Actor {
	return Actor{Type: ActorSystem, ID: ActorOperator}
}

// UserActor is a signed-in account, named by its public id.
func UserActor(userID string) Actor {
	return Actor{Type: ActorUser, ID: userID}
}
