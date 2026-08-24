- Session files and the session index are now replaced atomically, so a crash,
  a full disk, or a machine failure part-way through a save leaves the previous
  conversation intact instead of an unreadable file.
