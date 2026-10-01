package domain

// Diagnostic bounds are UTF-8 bytes, matching existing failure message limits.
func (f Failure) Validate() error {
	if f.Code == "" || len(f.Code) > 256 || len(f.Message) > 8192 || len(f.ExceptionType) > 1024 || len(f.StackTrace) > 65536 {
		return Bad("failure exceeds diagnostic limits (code 256, message 8192, exceptionType 1024, stackTrace 65536 UTF-8 bytes)")
	}
	switch f.Phase {
	case "", "initialization", "engine", "handler", "result", "completion":
	default:
		return Bad("invalid failure phase")
	}
	return nil
}
