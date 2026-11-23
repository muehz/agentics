package argv

// ShellKind says whether a command name is a shell and which grammar its text follows.
type ShellKind uint8

const (
	// NotShell is every name that is not a shell.
	NotShell ShellKind = iota
	// PosixShell is a shell whose text the POSIX parser models: sh and bash.
	PosixShell
	// OtherShell is a shell whose text no parser here models, so a call to one is never inspected.
	OtherShell
	// PowerShellHost is a PowerShell host, whose text the PowerShell parser models.
	PowerShellHost
)

// Command is what the vocabulary knows about one command name. Each attribute stands alone: a name carries only the
// attributes some guard or the script proof reads, and a name the table does not hold is the zero Command.
type Command struct {
	// Classified is true for a name some guard dispatches on.
	Classified bool
	// Shadow is true for a name a script may not redefine as a function, because a function of that name would
	// answer where a guard believes the real command runs.
	Shadow bool
	// VerbCLI is true for a command whose first argument selects the operation, so a parameter in that position
	// hands the caller the choice.
	VerbCLI bool
	// ProvenOperands is true for a command whose operands the script proof must prove before it allows the call.
	ProvenOperands bool
	// Evaluator is true for a command that runs text it is handed.
	Evaluator bool
	// Shell says whether the name is a shell, and of which kind.
	Shell ShellKind
}

// Lookup returns the entry for a command name, or the zero Command for a name the table does not hold. It folds no
// case and strips no path or extension, so callers pass the lower-cased executable name. The table is private: no
// consumer can add a name, and a test requires every row to be stated.
func Lookup(name string) Command {
	return vocabulary[name]
}

// vocabulary holds no attribute that names a writer: the writer commands stay a closed documented set with their
// guard. A row never has every attribute false.
var vocabulary = map[string]Command{
	// The deploy and release commands the release guard matches by name.
	"kubectl":                         {Classified: true, Shadow: true, VerbCLI: true},
	"helm":                            {Classified: true, Shadow: true, VerbCLI: true},
	"terragrunt":                      {Classified: true, Shadow: true, VerbCLI: true},
	"helmfile":                        {Classified: true, Shadow: true, VerbCLI: true},
	"terraform":                       {Classified: true, Shadow: true, VerbCLI: true},
	"tofu":                            {Classified: true, Shadow: true, VerbCLI: true},
	"pulumi":                          {Classified: true, Shadow: true, VerbCLI: true},
	"aws":                             {Classified: true, Shadow: true, VerbCLI: true},
	"az":                              {Classified: true, Shadow: true, VerbCLI: true},
	"new-azresourcegroupdeployment":   {Classified: true},
	"new-azdeployment":                {Classified: true},
	"new-azsubscriptiondeployment":    {Classified: true},
	"new-azmanagementgroupdeployment": {Classified: true},
	"new-aztenantdeployment":          {Classified: true},
	"gcloud":                          {Classified: true, Shadow: true, VerbCLI: true},
	"vercel":                          {Classified: true, Shadow: true, VerbCLI: true},

	// The commands the secrets guard treats as reading a file, and git, which it dispatches on to read a revision.
	"cat":           {Classified: true, Shadow: true},
	"bat":           {Classified: true, Shadow: true},
	"less":          {Classified: true, Shadow: true},
	"more":          {Classified: true, Shadow: true},
	"head":          {Classified: true, Shadow: true},
	"tail":          {Classified: true, Shadow: true},
	"strings":       {Classified: true, Shadow: true},
	"xxd":           {Classified: true, Shadow: true},
	"od":            {Classified: true, Shadow: true},
	"hexdump":       {Classified: true, Shadow: true},
	"base64":        {Classified: true, Shadow: true},
	"cp":            {Classified: true, Shadow: true},
	"mv":            {Classified: true, Shadow: true},
	"scp":           {Classified: true, Shadow: true},
	"rsync":         {Classified: true, Shadow: true},
	"sed":           {Classified: true, Shadow: true},
	"awk":           {Classified: true, Shadow: true},
	"grep":          {Classified: true, Shadow: true},
	"rg":            {Classified: true, Shadow: true},
	"ag":            {Classified: true, Shadow: true},
	"jq":            {Classified: true, Shadow: true},
	"yq":            {Classified: true, Shadow: true},
	"source":        {Classified: true, Shadow: true, Evaluator: true},
	".":             {Classified: true, Shadow: true, Evaluator: true},
	"dd":            {Classified: true, Shadow: true, ProvenOperands: true},
	"get-content":   {Classified: true},
	"gc":            {Classified: true},
	"type":          {Classified: true, Shadow: true},
	"select-string": {Classified: true},
	"copy-item":     {Classified: true},
	"copy":          {Classified: true},
	"format-hex":    {Classified: true},
	"git":           {Classified: true, Shadow: true, VerbCLI: true, ProvenOperands: true},

	// The commands the command guard checks for writing, deleting or moving a settings file or a Git hook.
	"tee":           {Classified: true, Shadow: true},
	"rm":            {Classified: true, Shadow: true, ProvenOperands: true},
	"del":           {Classified: true},
	"erase":         {Classified: true},
	"unlink":        {Classified: true, Shadow: true},
	"truncate":      {Classified: true, Shadow: true},
	"shred":         {Classified: true, Shadow: true},
	"move":          {Classified: true},
	"move-item":     {Classified: true},
	"mi":            {Classified: true},
	"remove-item":   {Classified: true},
	"ri":            {Classified: true},
	"set-content":   {Classified: true},
	"sc":            {Classified: true},
	"add-content":   {Classified: true},
	"ac":            {Classified: true},
	"clear-content": {Classified: true},
	"clc":           {Classified: true},
	"out-file":      {Classified: true},
	"rename-item":   {Classified: true},
	"rni":           {Classified: true},
	"ren":           {Classified: true},
	"new-item":      {Classified: true},
	"ni":            {Classified: true},
	"cpi":           {Classified: true},
	"install":       {Classified: true, Shadow: true},
	"ln":            {Classified: true, Shadow: true},
	":":             {Classified: true},

	// The commands the command guard reads for SQL text and for publication.
	"delete":  {Classified: true},
	"psql":    {Classified: true, Shadow: true},
	"mysql":   {Classified: true, Shadow: true},
	"mariadb": {Classified: true, Shadow: true},
	"sqlcmd":  {Classified: true, Shadow: true},
	"sqlite3": {Classified: true, Shadow: true},
	"gh":      {Classified: true, Shadow: true, VerbCLI: true},
	"npm":     {Classified: true, Shadow: true, VerbCLI: true},
	"dotnet":  {Classified: true, Shadow: true, VerbCLI: true},
	"nuget":   {Classified: true, Shadow: true, VerbCLI: true},

	// The downloaders and evaluators the command guard pairs across a pipe.
	"curl":              {Classified: true, Shadow: true},
	"wget":              {Classified: true, Shadow: true},
	"invoke-webrequest": {Classified: true},
	"iwr":               {Classified: true},
	"invoke-expression": {Classified: true, Evaluator: true},
	"iex":               {Classified: true, Evaluator: true},

	// The shells. The parser unwraps only the POSIX shells and the PowerShell hosts, and the script proof never inspects
	// a call to another shell. The command guard reads sh, bash and zsh as consumers of a download.
	"sh":         {Classified: true, Shadow: true, Shell: PosixShell},
	"bash":       {Classified: true, Shadow: true, Shell: PosixShell},
	"zsh":        {Classified: true, Shell: OtherShell},
	"dash":       {Shell: OtherShell},
	"ksh":        {Shell: OtherShell},
	"fish":       {Shell: OtherShell},
	"csh":        {Shell: OtherShell},
	"tcsh":       {Shell: OtherShell},
	"pwsh":       {Shadow: true, Shell: PowerShellHost},
	"powershell": {Shadow: true, Shell: PowerShellHost},

	// The names only the script proof reads: functions a script may not shadow, commands whose first argument
	// selects the operation, and commands that run text they are handed.
	"chmod":     {Shadow: true, ProvenOperands: true},
	"chown":     {Shadow: true},
	"mkfs":      {Shadow: true},
	"docker":    {Shadow: true, VerbCLI: true},
	"ssh":       {Shadow: true, Evaluator: true},
	"find":      {Shadow: true},
	"sort":      {Shadow: true},
	"uniq":      {Shadow: true},
	"wc":        {Shadow: true},
	"mkdir":     {Shadow: true},
	"sudo":      {Shadow: true},
	"doas":      {Shadow: true},
	"env":       {Shadow: true},
	"nohup":     {Shadow: true},
	"time":      {Shadow: true},
	"timeout":   {Shadow: true},
	"nice":      {Shadow: true},
	"ionice":    {Shadow: true},
	"stdbuf":    {Shadow: true},
	"setsid":    {Shadow: true},
	"flock":     {Shadow: true},
	"xargs":     {Shadow: true, Evaluator: true},
	"parallel":  {Shadow: true, Evaluator: true},
	"watch":     {Shadow: true, Evaluator: true},
	"echo":      {Shadow: true},
	"printf":    {Shadow: true},
	"mktemp":    {Shadow: true},
	"test":      {Shadow: true},
	"[":         {Shadow: true},
	"cd":        {Shadow: true},
	"eval":      {Shadow: true, Evaluator: true},
	"builtin":   {Shadow: true},
	"command":   {Shadow: true},
	"exec":      {Shadow: true},
	"read":      {Shadow: true},
	"set":       {Shadow: true},
	"unset":     {Shadow: true},
	"hash":      {Shadow: true},
	"enable":    {Shadow: true},
	"export":    {Shadow: true},
	"declare":   {Shadow: true},
	"typeset":   {Shadow: true},
	"local":     {Shadow: true},
	"readonly":  {Shadow: true},
	"alias":     {Shadow: true},
	"unalias":   {Shadow: true},
	"trap":      {Shadow: true},
	"pushd":     {Shadow: true},
	"popd":      {Shadow: true},
	"let":       {Shadow: true},
	"shopt":     {Shadow: true},
	"mapfile":   {Shadow: true},
	"readarray": {Shadow: true},
	"npx":       {VerbCLI: true},
	"pnpm":      {VerbCLI: true},
	"yarn":      {VerbCLI: true},
	"cargo":     {VerbCLI: true},
	"go":        {VerbCLI: true},
	"make":      {VerbCLI: true},
	"pip":       {VerbCLI: true},
}
