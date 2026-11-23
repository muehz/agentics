package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const (
	downloadExecuteSuggestion    = "Download the script to a file, inspect it, and run it only after verifying its contents."
	powerShellDownloadSuggestion = "Download the content to a file, inspect it, and run it only after verifying its contents."
	fileUploadSuggestion         = "Inspect the file and destination before sending data with 'curl --data @file'."
)

func checkTransferCommands(invocations []shellwords.Invocation, report reportFunc) {
	for i, inv := range invocations {
		if inv.Before == shellwords.Pipe || inv.PipelineInput {
			if producer, ok := pipelineProducer(invocations, i); ok && isDownloadProducer(producer) && isShellConsumerForPipeline(inv, producer) {
				report(ruleDownloadExecute, producer.Raw+" | "+inv.Raw, downloadExecuteSuggestion)
				return
			}
		}
		if inv.Parent >= 0 && inv.Parent < len(invocations) && (inv.Before == shellwords.Group || inv.Before == shellwords.Substitution) &&
			isPowerShellDownloader(inv) && isPowerShellEvaluator(invocations[inv.Parent]) {
			report(rulePowerShellDownload, invocations[inv.Parent].Raw+" ("+inv.Raw+")", powerShellDownloadSuggestion)
			return
		}
	}
	for _, inv := range invocations {
		if match, ok := curlUploadMatch(inv); ok {
			report(ruleFileUpload, match, fileUploadSuggestion)
			return
		}
	}
}

func isShellConsumerForPipeline(inv, producer shellwords.Invocation) bool {
	if len(inv.Words) > 0 && inv.Words[0].Literal {
		if shellConsumerName(strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))) {
			return true
		}
	}
	shared := 0
	if len(inv.WrapperIDs) == len(inv.Wrappers) && len(producer.WrapperIDs) == len(producer.Wrappers) {
		for shared < len(inv.WrapperIDs) && shared < len(producer.WrapperIDs) && inv.WrapperIDs[shared] != 0 && inv.WrapperIDs[shared] == producer.WrapperIDs[shared] {
			shared++
		}
	}
	return isShellConsumer(shellwords.Invocation{Wrappers: inv.Wrappers[shared:]})
}

// pipelineProducer finds the download invocation feeding consumerIndex,
// following the directional PipelineInputEdge / PipelineOutputEdge pairing
// assigned by the parser.
func pipelineProducer(invocations []shellwords.Invocation, consumerIndex int) (shellwords.Invocation, bool) {
	if consumerIndex < 0 || consumerIndex >= len(invocations) {
		return shellwords.Invocation{}, false
	}
	inputEdge := invocations[consumerIndex].PipelineInputEdge
	if inputEdge == 0 {
		return shellwords.Invocation{}, false
	}
	for i := consumerIndex - 1; i >= 0; i-- {
		if invocations[i].PipelineOutputEdge == inputEdge && invocations[i].PipelineOutput && isDownloadProducer(invocations[i]) {
			return invocations[i], true
		}
	}
	return shellwords.Invocation{}, false
}

func isDownloadProducer(inv shellwords.Invocation) bool {
	if !inv.Inspectable || len(inv.Words) == 0 || !inv.Words[0].Literal {
		return false
	}
	return downloadProducers[strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))]
}

// downloadProducers are the commands whose output a pipe can carry into a shell. Each name must be Classified in argv.
var downloadProducers = map[string]bool{
	"curl": true, "wget": true, "invoke-webrequest": true, "iwr": true,
}

func isShellConsumer(inv shellwords.Invocation) bool {
	if len(inv.Words) > 0 && inv.Words[0].Literal {
		if shellConsumerName(strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))) {
			return true
		}
	}
	for _, wrapper := range inv.Wrappers {
		if !wrapper.Literal {
			continue
		}
		if shellConsumerName(strings.ToLower(shellwords.ExecutableName(wrapper.Text))) {
			return true
		}
	}
	return false
}

// shellConsumerName reports whether a command of this lower-case name runs the text piped into it: every shell the
// vocabulary lists, and the PowerShell evaluator, which runs piped text without being a shell.
func shellConsumerName(name string) bool {
	return argv.Lookup(name).Shell != argv.NotShell || name == "iex" || name == "invoke-expression"
}

func isPowerShellDownloader(inv shellwords.Invocation) bool {
	if !inv.Inspectable || len(inv.Words) == 0 || !inv.Words[0].Literal {
		return false
	}
	name := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
	return name == "invoke-webrequest" || name == "iwr"
}

func isPowerShellEvaluator(inv shellwords.Invocation) bool {
	if !inv.Inspectable || len(inv.Words) == 0 || !inv.Words[0].Literal {
		return false
	}
	name := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
	return name == "invoke-expression" || name == "iex"
}

// curlOptions is the curl grammar the upload rule reads: every option whose
// next word is data, so a value that looks like an upload option is never read
// as one, and every flag letter that may precede an upload letter in a
// cluster. A letter outside the table ends its cluster unread. The table stays
// with this guard because it decides only what this rule skips.
var curlOptions = []argv.Option{
	{Name: "-E", Arity: argv.ArityValue}, {Name: "-K", Arity: argv.ArityValue}, {Name: "-C", Arity: argv.ArityValue},
	{Name: "-b", Arity: argv.ArityValue}, {Name: "-c", Arity: argv.ArityValue}, {Name: "-d", Arity: argv.ArityValue},
	{Name: "-D", Arity: argv.ArityValue}, {Name: "-F", Arity: argv.ArityValue}, {Name: "-P", Arity: argv.ArityValue},
	{Name: "-H", Arity: argv.ArityValue}, {Name: "-h", Arity: argv.ArityValue}, {Name: "-m", Arity: argv.ArityValue},
	{Name: "-o", Arity: argv.ArityValue}, {Name: "-x", Arity: argv.ArityValue}, {Name: "-U", Arity: argv.ArityValue},
	{Name: "-Q", Arity: argv.ArityValue}, {Name: "-r", Arity: argv.ArityValue}, {Name: "-e", Arity: argv.ArityValue},
	{Name: "-X", Arity: argv.ArityValue}, {Name: "-Y", Arity: argv.ArityValue}, {Name: "-y", Arity: argv.ArityValue},
	{Name: "-t", Arity: argv.ArityValue}, {Name: "-z", Arity: argv.ArityValue}, {Name: "-T", Arity: argv.ArityValue},
	{Name: "-u", Arity: argv.ArityValue}, {Name: "-A", Arity: argv.ArityValue}, {Name: "-w", Arity: argv.ArityValue},

	{Name: "--abstract-unix-socket", Arity: argv.ArityValue}, {Name: "--alt-svc", Arity: argv.ArityValue},
	{Name: "--aws-sigv4", Arity: argv.ArityValue}, {Name: "--cacert", Arity: argv.ArityValue},
	{Name: "--capath", Arity: argv.ArityValue}, {Name: "--cert", Arity: argv.ArityValue},
	{Name: "--cert-type", Arity: argv.ArityValue}, {Name: "--ciphers", Arity: argv.ArityValue},
	{Name: "--config", Arity: argv.ArityValue}, {Name: "--connect-timeout", Arity: argv.ArityValue},
	{Name: "--connect-to", Arity: argv.ArityValue}, {Name: "--continue-at", Arity: argv.ArityValue},
	{Name: "--cookie", Arity: argv.ArityValue}, {Name: "--cookie-jar", Arity: argv.ArityValue},
	{Name: "--create-file-mode", Arity: argv.ArityValue}, {Name: "--crlfile", Arity: argv.ArityValue},
	{Name: "--curves", Arity: argv.ArityValue}, {Name: "--data", Arity: argv.ArityValue},
	{Name: "--data-ascii", Arity: argv.ArityValue}, {Name: "--data-binary", Arity: argv.ArityValue},
	{Name: "--data-raw", Arity: argv.ArityValue}, {Name: "--data-urlencode", Arity: argv.ArityValue},
	{Name: "--delegation", Arity: argv.ArityValue}, {Name: "--dns-interface", Arity: argv.ArityValue},
	{Name: "--dns-ipv4-addr", Arity: argv.ArityValue}, {Name: "--dns-ipv6-addr", Arity: argv.ArityValue},
	{Name: "--dns-servers", Arity: argv.ArityValue}, {Name: "--doh-url", Arity: argv.ArityValue},
	{Name: "--dump-header", Arity: argv.ArityValue}, {Name: "--egd-file", Arity: argv.ArityValue},
	{Name: "--engine", Arity: argv.ArityValue}, {Name: "--etag-compare", Arity: argv.ArityValue},
	{Name: "--etag-save", Arity: argv.ArityValue}, {Name: "--expect100-timeout", Arity: argv.ArityValue},
	{Name: "--form", Arity: argv.ArityValue}, {Name: "--form-string", Arity: argv.ArityValue},
	{Name: "--ftp-account", Arity: argv.ArityValue}, {Name: "--ftp-alternative-to-user", Arity: argv.ArityValue},
	{Name: "--ftp-method", Arity: argv.ArityValue}, {Name: "--ftp-port", Arity: argv.ArityValue},
	{Name: "--ftp-ssl-ccc-mode", Arity: argv.ArityValue}, {Name: "--happy-eyeballs-timeout-ms", Arity: argv.ArityValue},
	{Name: "--haproxy-clientip", Arity: argv.ArityValue}, {Name: "--header", Arity: argv.ArityValue},
	{Name: "--help", Arity: argv.ArityValue}, {Name: "--hostpubmd5", Arity: argv.ArityValue},
	{Name: "--hostpubsha256", Arity: argv.ArityValue}, {Name: "--hsts", Arity: argv.ArityValue},
	{Name: "--interface", Arity: argv.ArityValue}, {Name: "--ipfs-gateway", Arity: argv.ArityValue},
	{Name: "--json", Arity: argv.ArityValue}, {Name: "--keepalive-time", Arity: argv.ArityValue},
	{Name: "--key", Arity: argv.ArityValue}, {Name: "--key-type", Arity: argv.ArityValue},
	{Name: "--krb", Arity: argv.ArityValue}, {Name: "--libcurl", Arity: argv.ArityValue},
	{Name: "--limit-rate", Arity: argv.ArityValue}, {Name: "--local-port", Arity: argv.ArityValue},
	{Name: "--login-options", Arity: argv.ArityValue}, {Name: "--mail-auth", Arity: argv.ArityValue},
	{Name: "--mail-from", Arity: argv.ArityValue}, {Name: "--mail-rcpt", Arity: argv.ArityValue},
	{Name: "--max-filesize", Arity: argv.ArityValue}, {Name: "--max-redirs", Arity: argv.ArityValue},
	{Name: "--max-time", Arity: argv.ArityValue}, {Name: "--netrc-file", Arity: argv.ArityValue},
	{Name: "--noproxy", Arity: argv.ArityValue}, {Name: "--oauth2-bearer", Arity: argv.ArityValue},
	{Name: "--output", Arity: argv.ArityValue}, {Name: "--output-dir", Arity: argv.ArityValue},
	{Name: "--parallel-max", Arity: argv.ArityValue}, {Name: "--pass", Arity: argv.ArityValue},
	{Name: "--pinnedpubkey", Arity: argv.ArityValue}, {Name: "--preproxy", Arity: argv.ArityValue},
	{Name: "--proto", Arity: argv.ArityValue}, {Name: "--proto-default", Arity: argv.ArityValue},
	{Name: "--proto-redir", Arity: argv.ArityValue}, {Name: "--proxy", Arity: argv.ArityValue},
	{Name: "--proxy-cacert", Arity: argv.ArityValue}, {Name: "--proxy-capath", Arity: argv.ArityValue},
	{Name: "--proxy-cert", Arity: argv.ArityValue}, {Name: "--proxy-cert-type", Arity: argv.ArityValue},
	{Name: "--proxy-ciphers", Arity: argv.ArityValue}, {Name: "--proxy-crlfile", Arity: argv.ArityValue},
	{Name: "--proxy-header", Arity: argv.ArityValue}, {Name: "--proxy-key", Arity: argv.ArityValue},
	{Name: "--proxy-key-type", Arity: argv.ArityValue}, {Name: "--proxy-pass", Arity: argv.ArityValue},
	{Name: "--proxy-pinnedpubkey", Arity: argv.ArityValue}, {Name: "--proxy-service-name", Arity: argv.ArityValue},
	{Name: "--proxy-tls13-ciphers", Arity: argv.ArityValue}, {Name: "--proxy-tlsauthtype", Arity: argv.ArityValue},
	{Name: "--proxy-tlspassword", Arity: argv.ArityValue}, {Name: "--proxy-tlsuser", Arity: argv.ArityValue},
	{Name: "--proxy-user", Arity: argv.ArityValue}, {Name: "--proxy1.0", Arity: argv.ArityValue},
	{Name: "--pubkey", Arity: argv.ArityValue}, {Name: "--quote", Arity: argv.ArityValue},
	{Name: "--random-file", Arity: argv.ArityValue}, {Name: "--range", Arity: argv.ArityValue},
	{Name: "--rate", Arity: argv.ArityValue}, {Name: "--referer", Arity: argv.ArityValue},
	{Name: "--request", Arity: argv.ArityValue}, {Name: "--request-target", Arity: argv.ArityValue},
	{Name: "--resolve", Arity: argv.ArityValue}, {Name: "--retry", Arity: argv.ArityValue},
	{Name: "--retry-delay", Arity: argv.ArityValue}, {Name: "--retry-max-time", Arity: argv.ArityValue},
	{Name: "--sasl-authzid", Arity: argv.ArityValue}, {Name: "--service-name", Arity: argv.ArityValue},
	{Name: "--socks4", Arity: argv.ArityValue}, {Name: "--socks4a", Arity: argv.ArityValue},
	{Name: "--socks5", Arity: argv.ArityValue}, {Name: "--socks5-gssapi-service", Arity: argv.ArityValue},
	{Name: "--socks5-hostname", Arity: argv.ArityValue}, {Name: "--speed-limit", Arity: argv.ArityValue},
	{Name: "--speed-time", Arity: argv.ArityValue}, {Name: "--stderr", Arity: argv.ArityValue},
	{Name: "--telnet-option", Arity: argv.ArityValue}, {Name: "--tftp-blksize", Arity: argv.ArityValue},
	{Name: "--time-cond", Arity: argv.ArityValue}, {Name: "--tls-max", Arity: argv.ArityValue},
	{Name: "--tls13-ciphers", Arity: argv.ArityValue}, {Name: "--tlsauthtype", Arity: argv.ArityValue},
	{Name: "--tlspassword", Arity: argv.ArityValue}, {Name: "--tlsuser", Arity: argv.ArityValue},
	{Name: "--trace", Arity: argv.ArityValue}, {Name: "--trace-ascii", Arity: argv.ArityValue},
	{Name: "--trace-config", Arity: argv.ArityValue}, {Name: "--unix-socket", Arity: argv.ArityValue},
	{Name: "--upload-file", Arity: argv.ArityValue}, {Name: "--url", Arity: argv.ArityValue},
	{Name: "--url-query", Arity: argv.ArityValue}, {Name: "--user", Arity: argv.ArityValue},
	{Name: "--user-agent", Arity: argv.ArityValue}, {Name: "--variable", Arity: argv.ArityValue},
	{Name: "--write-out", Arity: argv.ArityValue},

	{Name: "-a"}, {Name: "-q"}, {Name: "-f"}, {Name: "-s"}, {Name: "-S"}, {Name: "-k"}, {Name: "-L"}, {Name: "-I"},
	{Name: "-G"}, {Name: "-g"}, {Name: "-i"}, {Name: "-j"}, {Name: "-l"}, {Name: "-M"}, {Name: "-:"}, {Name: "-N"},
	{Name: "-Z"}, {Name: "-#"}, {Name: "-p"}, {Name: "-J"}, {Name: "-O"}, {Name: "-R"}, {Name: "-0"}, {Name: "-2"},
	{Name: "-3"}, {Name: "-1"}, {Name: "-4"}, {Name: "-6"}, {Name: "-B"}, {Name: "-v"}, {Name: "-V"},
}

// curlSpec matches names exactly, as curl does, and reads a word that comes
// from an expansion by its text, because an upload named through a variable
// is still an upload.
var curlSpec = argv.NewSpec(curlOptions...).With(argv.Clusters, argv.ReadNonLiteral)

// curlUploadMatch reports the first option that sends a local file and the
// text that names it. An unreadable word (an unknown option or cluster letter,
// a word from an expansion, a value option with no value) is skipped and the
// scan goes on: the scan's problem is never taken to mean "nothing found",
// and never stops the search for a later upload option. A `--` ends the
// search, because curl reads every word after it as a URL.
func curlUploadMatch(inv shellwords.Invocation) (string, bool) {
	if !inv.Inspectable || len(inv.Words) < 2 || !inv.Words[0].Literal || !strings.EqualFold(shellwords.ExecutableName(inv.Words[0].Text), "curl") {
		return "", false
	}
	scan := curlSpec.Scan(inv.Words[1:])
	end := scan.End()
	for _, arg := range scan.Args {
		if end >= 0 && arg.At >= end {
			break
		}
		if arg.Role != argv.RoleOption || !arg.HasValue {
			continue
		}
		value := arg.Value.Text
		reads := false
		switch arg.Name {
		case "-T", "--upload-file":
			reads = isCurlFileOperand(value)
		case "-d", "--data", "--data-ascii", "--data-binary", "--data-urlencode", "--json":
			reads = curlDataReadsFile(arg.Name, value)
		case "-F", "--form":
			reads = curlFormReadsFile(value)
		}
		if !reads {
			continue
		}
		// The match names the words a reviewer needs to find the upload: the
		// option word when it carries its value or is a cluster, otherwise
		// the value word, with the option too for a file upload.
		switch {
		case arg.Joined || arg.Letters != "":
			return "curl " + arg.Word.Raw, true
		case arg.Name == "-T" || arg.Name == "--upload-file":
			return "curl " + arg.Word.Raw + " " + arg.Value.Raw, true
		default:
			return "curl " + arg.Value.Raw, true
		}
	}
	return "", false
}

func isCurlFileOperand(value string) bool {
	return value != "" && value != "-" && value != "."
}

func curlDataReadsFile(option, value string) bool {
	if option != "--data-urlencode" {
		return strings.HasPrefix(value, "@") && value != "@-"
	}
	// Curl treats an equals form as inline data, even when an @ appears
	// elsewhere in the operand. Without '=', @ and name@ forms name a file.
	if strings.Contains(value, "=") {
		return false
	}
	if at := strings.IndexByte(value, '@'); at >= 0 {
		return at+1 < len(value) && value[at+1:] != "-"
	}
	return false
}

func curlFormReadsFile(value string) bool {
	equals := strings.IndexByte(value, '=')
	if equals < 0 || equals+1 >= len(value) {
		return false
	}
	payload := value[equals+1:]
	if semi := strings.IndexByte(payload, ';'); semi >= 0 {
		payload = payload[:semi]
	}
	return (strings.HasPrefix(payload, "@") || strings.HasPrefix(payload, "<")) && payload != "@-" && payload != "<-"
}
