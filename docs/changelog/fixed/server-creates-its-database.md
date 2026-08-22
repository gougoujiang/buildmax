- The server creates the database named by `database.name` when the MySQL it
  connects to does not have it, instead of refusing to start. It is attempted
  only after the connection failed for that reason; an account without `CREATE`
  rights gets an error naming the statement to run by hand.
