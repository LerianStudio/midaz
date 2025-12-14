package barerecover

// Test cases for nobarerecover analyzer

func badBareRecover() {
	defer recover() // want "recover\\(\\) call should capture and log the panic value"
}

func badDiscardedRecover() {
	defer func() {
		_ = recover() // want "recover\\(\\) result is discarded with blank identifier"
	}()
}

// This should NOT trigger - proper usage
func okRecoverWithLogging() {
	defer func() {
		if r := recover(); r != nil {
			// properly captured and would be logged
			_ = r
		}
	}()
}
