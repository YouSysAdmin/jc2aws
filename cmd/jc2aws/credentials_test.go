package main

// Credential output tests are integration-level (require real filesystem or
// real stdout). The outputCredentials function is exercised through the
// headless and TUI integration paths. The formatCredentials helper that
// previously lived here was removed as part of the stdout-output refactor.
