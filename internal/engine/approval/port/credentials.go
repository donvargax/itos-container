package port

const CredentialExposure = "This profile grants credentials. Each listed file will be readable by the worker; approval does not limit the token's provider permissions."
const CredentialStaging = "Each run copies the current source bytes to memory-backed staging, retains the copy for that worker, and drops it at teardown; the original source is never written." //nolint:gosec // G101: a disclosure sentence shown to the person, not a credential
const CredentialOutput = "Workload output and traffic are not redacted."
const CredentialNetwork = "The worker can send credential bytes to any allowed host."
const CredentialLogs = "A refused hostname can contain credential bytes and is kept in proxy and host refusal logs; credential absence from those logs is not established."
const CredentialGate = "Credential delivery is not available in this increment; run still refuses version 3 before manifest, engine or host state access." //nolint:gosec // G101: a disclosure sentence shown to the person, not a credential

// CredentialHelp previews the prospective delivery and exposure limits.
func CredentialHelp() string {
	return "\nProspective credential delivery limits:\n" + CredentialExposure + "\n" + CredentialStaging + "\n" + CredentialOutput + "\n\nAllowlist credential profiles additionally have these limits:\n" + CredentialNetwork + "\n" + CredentialLogs + "\n\n" + CredentialGate + "\n"
}
