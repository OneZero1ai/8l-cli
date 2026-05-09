package profile

import "testing"

func TestProfileValidate(t *testing.T) {
	tests := []struct {
		name    string
		p       *Profile
		wantErr bool
	}{
		{
			name: "happy path",
			p: &Profile{
				Version:   1,
				ManagedBy: "8l join v0.1.0",
				ManagedAt: "2026-05-09T12:00:00Z",
				Binding:   Binding{Enterprise: "8th-layer-corp", L2: "engineering", Persona: "alice"},
				MCPServers: map[string]MCPServer{
					"cq": {
						Type:    "stdio",
						Command: "cq",
						Env: map[string]string{
							"CQ_ADDR":    "https://engineering.8th-layer-corp.8th-layer.ai",
							"CQ_API_KEY": "cqa.v1.0123456789abcdef0123456789abcdef.x",
						},
					},
				},
			},
		},
		{
			name: "missing version",
			p: &Profile{
				ManagedBy:  "8l join v0",
				Binding:    Binding{Enterprise: "e", L2: "l", Persona: "p"},
				MCPServers: map[string]MCPServer{"cq": {Type: "stdio", Command: "cq", Env: map[string]string{"CQ_ADDR": "x", "CQ_API_KEY": "y"}}},
			},
			wantErr: true,
		},
		{
			name: "future version refused",
			p: &Profile{
				Version:    99,
				ManagedBy:  "8l join future",
				Binding:    Binding{Enterprise: "e", L2: "l", Persona: "p"},
				MCPServers: map[string]MCPServer{"cq": {Type: "stdio", Command: "cq", Env: map[string]string{"CQ_ADDR": "x", "CQ_API_KEY": "y"}}},
			},
			wantErr: true,
		},
		{
			name: "managed_at not RFC3339",
			p: &Profile{
				Version:    1,
				ManagedBy:  "8l join v0.1.0",
				ManagedAt:  "yesterday",
				Binding:    Binding{Enterprise: "8th-layer-corp", L2: "engineering", Persona: "alice"},
				MCPServers: map[string]MCPServer{"cq": {Type: "stdio", Command: "cq", Env: map[string]string{"CQ_ADDR": "x", "CQ_API_KEY": "y"}}},
			},
			wantErr: true,
		},
		{
			name: "invalid persona",
			p: &Profile{
				Version:    1,
				ManagedBy:  "8l join v0.1.0",
				Binding:    Binding{Enterprise: "e", L2: "l", Persona: "Alice With Spaces"},
				MCPServers: map[string]MCPServer{"cq": {Type: "stdio", Command: "cq", Env: map[string]string{"CQ_ADDR": "x", "CQ_API_KEY": "y"}}},
			},
			wantErr: true,
		},
		{
			name: "missing cq mcpServer",
			p: &Profile{
				Version:    1,
				ManagedBy:  "8l join v0.1.0",
				Binding:    Binding{Enterprise: "e", L2: "l", Persona: "p"},
				MCPServers: map[string]MCPServer{"other": {Type: "stdio", Command: "x", Env: map[string]string{"CQ_ADDR": "x", "CQ_API_KEY": "y"}}},
			},
			wantErr: true,
		},
		{
			name: "cq.env missing CQ_API_KEY",
			p: &Profile{
				Version:    1,
				ManagedBy:  "8l join v0.1.0",
				Binding:    Binding{Enterprise: "e", L2: "l", Persona: "p"},
				MCPServers: map[string]MCPServer{"cq": {Type: "stdio", Command: "cq", Env: map[string]string{"CQ_ADDR": "x"}}},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBindingEqual(t *testing.T) {
	a := Binding{Enterprise: "e", L2: "l", Persona: "p"}
	b := Binding{Enterprise: "e", L2: "l", Persona: "p"}
	c := Binding{Enterprise: "e", L2: "l", Persona: "q"}
	if !a.Equal(b) {
		t.Fatal("expected a.Equal(b)")
	}
	if a.Equal(c) {
		t.Fatal("expected !a.Equal(c)")
	}
}

func TestIsManagedBy8l(t *testing.T) {
	cases := map[string]bool{
		"8l join v0.1.0":   true,
		"8l join v9.99.99": true,
		"some-other-tool":  false,
		"":                 false,
		"hand-edited":      false,
		"8L Join v0.1.0":   false, // case-sensitive — protective
	}
	for mb, want := range cases {
		p := &Profile{ManagedBy: mb}
		if got := p.IsManagedBy8l(); got != want {
			t.Fatalf("managed_by=%q: got %v, want %v", mb, got, want)
		}
	}
}
