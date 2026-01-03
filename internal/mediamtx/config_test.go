package mediamtx

import (
	"sync"
	"testing"
)

func resetConfigForTest() {
	globalConfig = &Config{}
	configOnce = sync.Once{}
}

func TestSetConfigSetsWrapperPath(t *testing.T) {
	resetConfigForTest()

	if globalConfig.UseSystemd != false {
		t.Fatalf("expected default UseSystemd to be false, got %v", globalConfig.UseSystemd)
	}

	SetConfig(&Config{UseSystemd: true})

	if globalConfig.UseSystemd != true {
		t.Fatalf("expected UseSystemd to be set, got %v", globalConfig.UseSystemd)
	}

	// Subsequent calls should be ignored due to sync.Once
	SetConfig(&Config{UseSystemd: false})

	if globalConfig.UseSystemd != true {
		t.Fatalf("expected UseSystemd to remain unchanged, got %v", globalConfig.UseSystemd)
	}
}

func TestSetConfigOnceWithConcurrency(t *testing.T) {
	resetConfigForTest()

	const goroutines = 10
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			SetConfig(&Config{UseSystemd: id%2 == 0})
		}(i)
	}

	wg.Wait()

	// Only one of the provided UseSystemd values should be applied.
	// We can't predict which one, but we can verify it doesn't change after

	// Capture the applied value and ensure further calls don't change it.
	applied := globalConfig.UseSystemd
	SetConfig(&Config{UseSystemd: !applied})

	if globalConfig.UseSystemd != applied {
		t.Fatalf("expected UseSystemd %v to remain after subsequent SetConfig", applied)
	}
}
