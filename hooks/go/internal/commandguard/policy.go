package commandguard

// The suggestions of the workflow-policy detections. A rule's declared setting,
// not its suggestion, decides which setting governs it, so these name text only.
const (
	npmPublishSuggestion           = "Use 'npm publish --dry-run' to inspect the package contents without publishing. If the user still wants to publish the package, tell the user that agents cannot publish packages and ask them to run 'npm publish' in their own terminal."
	nugetPublishSuggestion         = "NuGet package publication requires a human. Build and inspect the package with 'dotnet pack', then ask the user to run the package push in their own terminal."
	mcpCreatePullRequestSuggestion = "Set draft: true. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	mcpUpdatePullRequestSuggestion = "Do not set draft to false. Every other field of this tool is ungated. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	mcpDraftStateSuggestion        = "Leave the pull request's draft state unchanged. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	reviewThreadSuggestion         = "Leave the review thread's resolution state unchanged and draft the reply. Tell the user that agents cannot change review-thread resolution state and ask them to resolve or reopen the thread in the pull request UI, as appropriate."
	alertDismissalSuggestion       = "Leave the alert open and record the rationale in the drafted reply. If dismissal is still required, tell the user that agents cannot dismiss the alert and ask them to do so in the repository's security alerts UI."
)
