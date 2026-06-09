package catalog

type Catalog struct {
	Version int `toml:"version"`

	TargetGroups map[string]string `toml:"target_groups"`

	Hosts             []Host             `toml:"hosts"`
	Repos             []Repo             `toml:"repos"`
	CredentialSources []CredentialSource `toml:"credential_sources"`
	AgentRuntimes     []AgentRuntime     `toml:"agent_runtimes"`
	HarnessConfigs    []HarnessConfig    `toml:"harness_configs"`
	HarnessExtensions []HarnessExtension `toml:"harness_extensions"`
	SkillPacks        []SkillPack        `toml:"skill_packs"`
	Integrations      []Integration      `toml:"integrations"`
	AgentWorkloads    []AgentWorkload    `toml:"agent_workloads"`
	Automations       []LegacyAutomation `toml:"automations"`
	AuxServices       []AuxService       `toml:"aux_services"`
	StateStores       []StateStore       `toml:"state_stores"`
	DataSources       []DataSource       `toml:"data_sources"`
	CredentialProbes  []CredentialProbe  `toml:"credential_probes"`
}

type Host struct {
	ID           string   `toml:"id"`
	Hostname     string   `toml:"hostname"`
	SSHAlias     string   `toml:"ssh_alias"`
	Aliases      []string `toml:"aliases"`
	TargetGroups []string `toml:"target_groups"`
	Roles        []string `toml:"roles"`
	Notes        string   `toml:"notes"`
	sourceDir    string
}

type Repo struct {
	ID           string   `toml:"id"`
	Remote       string   `toml:"remote"`
	Path         string   `toml:"path"`
	Branch       string   `toml:"branch"`
	UpdatePolicy string   `toml:"update_policy"`
	AuthRef      string   `toml:"auth_ref"`
	Targets      []string `toml:"targets"`
	Required     bool     `toml:"required"`
	Notes        string   `toml:"notes"`
	sourceDir    string
}

type CredentialSource struct {
	ID        string   `toml:"id"`
	Type      string   `toml:"type"`
	Status    string   `toml:"status"`
	Env       string   `toml:"env"`
	Username  string   `toml:"username"`
	Targets   []string `toml:"targets"`
	Notes     string   `toml:"notes"`
	sourceDir string
}

type AgentRuntime struct {
	ID        string   `toml:"id"`
	Type      string   `toml:"type"`
	Status    string   `toml:"status"`
	Version   string   `toml:"version"`
	Command   string   `toml:"command"`
	Targets   []string `toml:"targets"`
	Notes     string   `toml:"notes"`
	sourceDir string
}

type HarnessConfig struct {
	ID        string            `toml:"id"`
	Harness   string            `toml:"harness"`
	Type      string            `toml:"type"`
	Status    string            `toml:"status"`
	Path      string            `toml:"path"`
	Settings  map[string]string `toml:"settings"`
	Targets   []string          `toml:"targets"`
	Notes     string            `toml:"notes"`
	sourceDir string
}

type HarnessExtension struct {
	ID        string   `toml:"id"`
	Type      string   `toml:"type"`
	Status    string   `toml:"status"`
	Source    string   `toml:"source"`
	Path      string   `toml:"path"`
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	Harnesses []string `toml:"harnesses"`
	Targets   []string `toml:"targets"`
	Notes     string   `toml:"notes"`
	sourceDir string
}

type SkillPack struct {
	ID                   string   `toml:"id"`
	Source               string   `toml:"source"`
	InstallPath          string   `toml:"install_path"`
	CompatibilityAliases []string `toml:"compatibility_aliases"`
	Harnesses            []string `toml:"harnesses"`
	Targets              []string `toml:"targets"`
	Notes                string   `toml:"notes"`
	sourceDir            string
}

type Integration struct {
	ID                  string            `toml:"id"`
	Type                string            `toml:"type"`
	Status              string            `toml:"status"`
	Description         string            `toml:"description"`
	Command             string            `toml:"command"`
	Args                []string          `toml:"args"`
	URL                 string            `toml:"url"`
	Provider            string            `toml:"provider"`
	Headers             map[string]string `toml:"headers"`
	EnvRefs             map[string]string `toml:"env_refs"`
	Harnesses           []string          `toml:"harnesses"`
	AllowedTools        []string          `toml:"allowed_tools"`
	ExtensionRef        string            `toml:"extension_ref"`
	AuxServiceRef       string            `toml:"aux_service_ref"`
	DataSourceRefs      []string          `toml:"data_source_refs"`
	CredentialProbeRefs []string          `toml:"credential_probe_refs"`
	Targets             []string          `toml:"targets"`
	Notes               string            `toml:"notes"`
	sourceDir           string
}

type AgentWorkload struct {
	ID                   string            `toml:"id"`
	Name                 string            `toml:"name"`
	Owner                string            `toml:"owner"`
	Kind                 string            `toml:"kind"`
	CodexKind            string            `toml:"codex_kind"`
	Status               string            `toml:"status"`
	Schedule             string            `toml:"schedule"`
	Timezone             string            `toml:"timezone"`
	Targets              []string          `toml:"targets"`
	Harnesses            []string          `toml:"harnesses"`
	Command              string            `toml:"command"`
	Args                 []string          `toml:"args"`
	CWD                  string            `toml:"cwd"`
	EnvRefs              map[string]string `toml:"env_refs"`
	LogPath              string            `toml:"log_path"`
	HealthCommand        string            `toml:"health_command"`
	RestartPolicy        string            `toml:"restart_policy"`
	Prompt               string            `toml:"prompt"`
	PromptFile           string            `toml:"prompt_file"`
	Skill                string            `toml:"skill"`
	Model                string            `toml:"model"`
	Reasoning            string            `toml:"reasoning"`
	Delivery             string            `toml:"delivery"`
	RunPolicy            string            `toml:"run_policy"`
	ExecutionEnvironment string            `toml:"execution_environment"`
	TargetThreadID       string            `toml:"target_thread_id"`
	Tags                 []string          `toml:"tags"`
	Capabilities         []string          `toml:"capabilities"`
	IntegrationRefs      []string          `toml:"integration_refs"`
	StateStoreRefs       []string          `toml:"state_store_refs"`
	SourceRefs           []string          `toml:"source_refs"`
	Notes                string            `toml:"notes"`
	sourceDir            string
}

type LegacyAutomation AgentWorkload

type AuxService struct {
	ID             string            `toml:"id"`
	Type           string            `toml:"type"`
	Status         string            `toml:"status"`
	Description    string            `toml:"description"`
	Command        string            `toml:"command"`
	Args           []string          `toml:"args"`
	CWD            string            `toml:"cwd"`
	EnvRefs        map[string]string `toml:"env_refs"`
	URL            string            `toml:"url"`
	LogPath        string            `toml:"log_path"`
	HealthCommand  string            `toml:"health_command"`
	RestartPolicy  string            `toml:"restart_policy"`
	Schedule       string            `toml:"schedule"`
	StateStoreRefs []string          `toml:"state_store_refs"`
	Targets        []string          `toml:"targets"`
	Notes          string            `toml:"notes"`
	sourceDir      string
}

type StateStore struct {
	ID        string   `toml:"id"`
	Type      string   `toml:"type"`
	Status    string   `toml:"status"`
	Path      string   `toml:"path"`
	URL       string   `toml:"url"`
	Targets   []string `toml:"targets"`
	Notes     string   `toml:"notes"`
	sourceDir string
}

type DataSource struct {
	ID          string   `toml:"id"`
	Type        string   `toml:"type"`
	Status      string   `toml:"status"`
	Description string   `toml:"description"`
	URL         string   `toml:"url"`
	Targets     []string `toml:"targets"`
	Notes       string   `toml:"notes"`
	sourceDir   string
}

type CredentialProbe struct {
	ID             string   `toml:"id"`
	Type           string   `toml:"type"`
	Status         string   `toml:"status"`
	Description    string   `toml:"description"`
	Command        string   `toml:"command"`
	Args           []string `toml:"args"`
	Path           string   `toml:"path"`
	Env            string   `toml:"env"`
	IntegrationRef string   `toml:"integration_ref"`
	DataSourceRef  string   `toml:"data_source_ref"`
	Targets        []string `toml:"targets"`
	Notes          string   `toml:"notes"`
	sourceDir      string
}

func (w AgentWorkload) DisplayName() string {
	if w.Name != "" {
		return w.Name
	}
	return w.ID
}

func (w AgentWorkload) SourceDir() string { return w.sourceDir }
func (a AuxService) SourceDir() string    { return a.sourceDir }
func (i Integration) SourceDir() string   { return i.sourceDir }
func (s SkillPack) SourceDir() string     { return s.sourceDir }
func (h HarnessExtension) SourceDir() string {
	return h.sourceDir
}
