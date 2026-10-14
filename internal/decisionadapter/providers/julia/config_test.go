package julia

import (
	"reflect"
	"testing"
	"time"
)

func TestConfigFromEnv(t *testing.T) {
	// juliaEnvKeys are the keys ConfigFromEnv reads from the environment. They
	// are neutralized (set to the empty string) before every subtest so the
	// table is hermetic on hosts that export JULIA_* configuration (for example
	// a workstation with a locally configured Julia provider). ConfigFromEnv
	// treats empty values as unset, so the neutralized keys fall back to the
	// documented defaults.
	juliaEnvKeys := []string{
		"JULIA_MODEL",
		"JULIA_MODEL_PATH",
		"JULIA_PYTHON",
		"JULIA_INFERENCE_CMD",
		"JULIA_TIMEOUT",
	}

	tests := map[string]struct {
		env  map[string]string
		want Config
	}{
		"all unset uses defaults": {
			env:  map[string]string{},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"model set": {
			env:  map[string]string{"JULIA_MODEL": "julia-risk"},
			want: Config{Model: "julia-risk", Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"python set": {
			env:  map[string]string{"JULIA_PYTHON": "python3.12"},
			want: Config{Model: DefaultModel, Python: "python3.12", Timeout: DefaultTimeout},
		},
		"inference command split": {
			env: map[string]string{"JULIA_INFERENCE_CMD": "python infer.py --model m.onnx"},
			want: Config{
				Model:        DefaultModel,
				Python:       DefaultPython,
				InferenceCmd: []string{"python", "infer.py", "--model", "m.onnx"},
				Timeout:      DefaultTimeout,
			},
		},
		"blank inference command unavailable": {
			env:  map[string]string{"JULIA_INFERENCE_CMD": "   "},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"model path set": {
			env:  map[string]string{"JULIA_MODEL_PATH": "/models/julia.onnx"},
			want: Config{Model: DefaultModel, ModelPath: "/models/julia.onnx", Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"timeout set": {
			env:  map[string]string{"JULIA_TIMEOUT": "5s"},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: 5 * time.Second},
		},
		"invalid timeout falls back": {
			env:  map[string]string{"JULIA_TIMEOUT": "not-a-duration"},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"non-positive timeout falls back": {
			env:  map[string]string{"JULIA_TIMEOUT": "-5s"},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, key := range juliaEnvKeys {
				t.Setenv(key, "")
			}
			for key, val := range tt.env {
				t.Setenv(key, val)
			}
			got := ConfigFromEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConfigFromEnv() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConfigWithDefaults(t *testing.T) {
	tests := map[string]struct {
		in   Config
		want Config
	}{
		"zero value": {
			in:   Config{},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
		"preserves set values": {
			in:   Config{Model: "custom", ModelPath: "/m.onnx", Python: "python3.12", InferenceCmd: []string{"infer"}, Timeout: 3 * time.Second},
			want: Config{Model: "custom", ModelPath: "/m.onnx", Python: "python3.12", InferenceCmd: []string{"infer"}, Timeout: 3 * time.Second},
		},
		"blank model and non-positive timeout": {
			in:   Config{Model: "  ", Timeout: -1},
			want: Config{Model: DefaultModel, Python: DefaultPython, Timeout: DefaultTimeout},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.in.WithDefaults()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithDefaults() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConfigWithDefaultsDoesNotMutateReceiver(t *testing.T) {
	orig := Config{}
	_ = orig.WithDefaults()
	if orig.Model != "" || orig.Timeout != 0 {
		t.Errorf("WithDefaults mutated receiver: %+v", orig)
	}
}
