package entity

import "buildmax/internal/model"

// Type aliases to shared domain types.
//
// This keeps store interfaces stable while allowing other layers to depend on
// internal/model for the concrete data shapes.
type User = model.User
type Workspace = model.Workspace
type Chat = model.Chat
type ChatRun = model.ChatRun
type ChatRunOutputFile = model.ChatRunOutputFile
type Agent = model.Agent
type ArtifactWithChat = model.ArtifactWithChat
