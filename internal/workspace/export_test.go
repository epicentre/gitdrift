package workspace

// SetOnAcquire installs the test hook that runs once a refresh holds both the
// semaphore slot and the group lock.
func SetOnAcquire(w *Workspace, fn func(active int)) { w.onAcquire = fn }
