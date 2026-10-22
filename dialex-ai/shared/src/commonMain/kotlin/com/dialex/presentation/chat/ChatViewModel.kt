package com.dialex.presentation.chat

import androidx.lifecycle.viewModelScope
import com.dialex.domain.repository.DiscussionRepository
import com.dialex.domain.usecase.ConductSocraticTurnUseCase
import com.dialex.domain.usecase.DuplicateDiscussionUseCase
import com.dialex.domain.usecase.ElevateSocraticToCouncilUseCase
import com.dialex.domain.usecase.GenerateDeliverableFormatUseCase
import com.dialex.domain.usecase.GenerateDiscussionTitleUseCase
import com.dialex.domain.usecase.GenerateHandoffPromptUseCase
import com.dialex.domain.usecase.GenerateSocraticDigestUseCase
import com.dialex.domain.usecase.GetDiscussionUsageUseCase
import com.dialex.domain.usecase.GetDiscussionUseCase
import com.dialex.domain.usecase.PauseDiscussionUseCase
import com.dialex.domain.usecase.RecalculateCredenceUseCase
import com.dialex.domain.usecase.ResumeDiscussionUseCase
import com.dialex.domain.usecase.StopDiscussionUseCase
import com.dialex.domain.usecase.StreamDiscussionUseCase
import com.dialex.domain.usecase.UpdateDiscussionUseCase
import com.dialex.export.exportFileName
import com.dialex.export.toMarkdown
import com.dialex.model.Discussion
import com.dialex.model.DiscussionStatus
import com.dialex.model.Provider
import com.dialex.model.isFrom
import com.dialex.model.label
import com.dialex.presentation.base.MviViewModel
import com.dialex.presentation.chat.components.resolveAllArtifacts
import com.dialex.presentation.setup.TokenWarningLevel
import com.dialex.util.formatTokenCount
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.toImmutableList
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

class ChatViewModel(
    private val getDiscussionUseCase: GetDiscussionUseCase,
    private val streamDiscussionUseCase: StreamDiscussionUseCase,
    private val updateDiscussionUseCase: UpdateDiscussionUseCase,
    private val pauseDiscussionUseCase: PauseDiscussionUseCase,
    private val resumeDiscussionUseCase: ResumeDiscussionUseCase,
    private val stopDiscussionUseCase: StopDiscussionUseCase,
    private val getDiscussionUsageUseCase: GetDiscussionUsageUseCase,
    private val duplicateDiscussionUseCase: DuplicateDiscussionUseCase,
    private val generateDiscussionTitleUseCase: GenerateDiscussionTitleUseCase,
    private val generateDeliverableFormatUseCase: GenerateDeliverableFormatUseCase,
    private val generateHandoffPromptUseCase: GenerateHandoffPromptUseCase,
    private val discussionId: String,
    private val tokenBudget: Int,
    private val conductSocraticTurnUseCase: ConductSocraticTurnUseCase? = null,
    private val generateSocraticDigestUseCase: GenerateSocraticDigestUseCase? = null,
    private val elevateSocraticToCouncilUseCase: ElevateSocraticToCouncilUseCase? = null,
    private val recalculateCredenceUseCase: RecalculateCredenceUseCase? = null,
) : MviViewModel<ChatState, ChatIntent, ChatEffect>(ChatState()) {

    constructor(
        discussionRepository: DiscussionRepository,
        discussionId: String,
        tokenBudget: Int,
        conductSocraticTurnUseCase: ConductSocraticTurnUseCase? = null,
        generateSocraticDigestUseCase: GenerateSocraticDigestUseCase? = null,
        elevateSocraticToCouncilUseCase: ElevateSocraticToCouncilUseCase? = null,
        recalculateCredenceUseCase: RecalculateCredenceUseCase? = null,
    ) : this(
        getDiscussionUseCase = GetDiscussionUseCase(discussionRepository),
        streamDiscussionUseCase = StreamDiscussionUseCase(discussionRepository),
        updateDiscussionUseCase = UpdateDiscussionUseCase(discussionRepository),
        pauseDiscussionUseCase = PauseDiscussionUseCase(discussionRepository),
        resumeDiscussionUseCase = ResumeDiscussionUseCase(discussionRepository),
        stopDiscussionUseCase = StopDiscussionUseCase(discussionRepository),
        getDiscussionUsageUseCase = GetDiscussionUsageUseCase(discussionRepository),
        duplicateDiscussionUseCase = DuplicateDiscussionUseCase(discussionRepository),
        generateDiscussionTitleUseCase = GenerateDiscussionTitleUseCase(discussionRepository),
        generateDeliverableFormatUseCase = GenerateDeliverableFormatUseCase(discussionRepository),
        generateHandoffPromptUseCase = GenerateHandoffPromptUseCase(discussionRepository),
        discussionId = discussionId,
        tokenBudget = tokenBudget,
        conductSocraticTurnUseCase = conductSocraticTurnUseCase ?: ConductSocraticTurnUseCase(discussionRepository),
        generateSocraticDigestUseCase = generateSocraticDigestUseCase ?: GenerateSocraticDigestUseCase(discussionRepository),
        elevateSocraticToCouncilUseCase = elevateSocraticToCouncilUseCase ?: ElevateSocraticToCouncilUseCase(discussionRepository),
        recalculateCredenceUseCase = recalculateCredenceUseCase
    )

    private val socraticSessionDelegate = SocraticSessionDelegate(
        conductSocraticTurnUseCase = conductSocraticTurnUseCase,
        generateSocraticDigestUseCase = generateSocraticDigestUseCase,
        elevateSocraticToCouncilUseCase = elevateSocraticToCouncilUseCase,
        updateDiscussionUseCase = updateDiscussionUseCase
    )

    private var streamJob: Job? = null
    private var lastLoggedErrorMsg: String? = null

    init {
        viewModelScope.launch {
            try {
                val discussion = getDiscussionUseCase(discussionId)
                applyUpdatedDiscussion(discussion)
                if (discussion.mode == com.dialex.domain.model.DiscussionMode.SOCRATIC_INTERVIEW) {
                    val lastExaminerMsg = discussion.transcript.lastOrNull { !it.isUserComment && !it.isError && !it.isSystem }
                    setState {
                        copy(
                            socraticDigest = discussion.socraticDigest,
                            activeProbe = lastExaminerMsg?.content,
                            socraticStage = discussion.socraticConfig?.stage ?: com.dialex.domain.model.SocraticStage.HYPOTHESIS_EXTRACTION
                        )
                    }
                }
                setState { copy(loadAsync = AsyncState.Success(Unit)) }
                if (discussion.status == DiscussionStatus.RUNNING) {
                    startStreamingOrPolling()
                }
            } catch (e: Exception) {
                setState { copy(loadAsync = AsyncState.Error(e)) }
            }
        }
    }

    private fun applyUpdatedDiscussion(updatedDiscussion: Discussion) {
        val currentDisc = state.value.discussion
        val currentTranscript = currentDisc?.transcript.orEmpty()
        val cleanLocalTranscript = if (updatedDiscussion.status == DiscussionStatus.RUNNING) {
            currentTranscript.dropLastWhile { it.isError }
        } else {
            currentTranscript
        }
        val mergedTranscript = TranscriptMerger.mergeTranscripts(cleanLocalTranscript, updatedDiscussion.transcript, updatedDiscussion.config)
        val currentArts = currentDisc?.artifacts.orEmpty()
        val mergedArts = (updatedDiscussion.artifacts + currentArts).distinctBy { it.id }
        val currentDismissed = currentDisc?.dismissedArtifactIds.orEmpty()
        val mergedDismissed = (updatedDiscussion.dismissedArtifactIds + currentDismissed).distinct()
        val mergedDiscussion = updatedDiscussion.copy(
            transcript = mergedTranscript,
            artifacts = mergedArts,
            dismissedArtifactIds = mergedDismissed,
            deliverable = if (updatedDiscussion.status == DiscussionStatus.RUNNING) (currentDisc?.deliverable ?: updatedDiscussion.deliverable) else updatedDiscussion.deliverable,
            conclusion = if (updatedDiscussion.status == DiscussionStatus.RUNNING) (currentDisc?.conclusion ?: updatedDiscussion.conclusion) else updatedDiscussion.conclusion,
            summary = if (updatedDiscussion.status == DiscussionStatus.RUNNING) (currentDisc?.summary ?: updatedDiscussion.summary) else (updatedDiscussion.summary ?: updatedDiscussion.conclusion)
        )

        if (updatedDiscussion.status == DiscussionStatus.ERROR) {
            val errTurn = updatedDiscussion.transcript.findLast { it.isError }
            val errMsg = errTurn?.content ?: "Debate reached ERROR state"
            if (lastLoggedErrorMsg != errMsg) {
                lastLoggedErrorMsg = errMsg
                com.dialex.logging.AppLogStore.error("DebateRunner", "Debate error: $errMsg")
            }
        } else if (updatedDiscussion.status == DiscussionStatus.RUNNING) {
            lastLoggedErrorMsg = null
        }

        val activeTensions = (updatedDiscussion.tensionPairs.ifEmpty { currentDisc?.tensionPairs.orEmpty() }).toImmutableList()
        val activeEvidence = (updatedDiscussion.retrievedEvidence.ifEmpty { currentDisc?.retrievedEvidence.orEmpty() }).toImmutableList()
        val activeCredence = updatedDiscussion.credenceLedger ?: currentDisc?.credenceLedger
        setState { copy(discussion = mergedDiscussion, tensionPairs = activeTensions, retrievedEvidence = activeEvidence, credenceLedger = activeCredence, isActionInProgress = false) }
        computeTokenWarning(mergedDiscussion)
        updateNextSpeaker(mergedDiscussion)
        validateWorkspaceFolders(mergedDiscussion)
    }

    private fun startStreamingOrPolling() {
        streamJob?.cancel()
        streamJob = viewModelScope.launch {
            while (isActive) {
                try {
                    streamDiscussionUseCase(discussionId)
                        .onEach { updatedDiscussion ->
                            // Stream is alive — clear any prior connection-lost flag.
                            if (state.value.engineConnectionLost) {
                                setState { copy(engineConnectionLost = false) }
                            }
                            applyUpdatedDiscussion(updatedDiscussion)
                            if (updatedDiscussion.status != DiscussionStatus.RUNNING) {
                                return@onEach
                            }
                        }
                        .collect()
                } catch (e: Exception) {
                    // Stream closed or error - will fallback to polling if still RUNNING
                }

                // If discussion is no longer RUNNING, stop the streaming/polling loop immediately
                val currentStatus = state.value.discussion?.status
                if (currentStatus != null && currentStatus != DiscussionStatus.RUNNING) {
                    break
                }

                // If stream finishes or drops while still RUNNING, fetch latest snapshot
                val latest = runCatching { getDiscussionUseCase(discussionId) }.getOrNull()
                if (latest != null) {
                    // Poll succeeded — engine is reachable again.
                    if (state.value.engineConnectionLost) {
                        setState { copy(engineConnectionLost = false) }
                    }
                    applyUpdatedDiscussion(latest)
                    if (latest.status != DiscussionStatus.RUNNING) {
                        break
                    }
                    delay(400)
                } else {
                    // Both stream and poll failed — engine is unreachable.
                    if (!state.value.engineConnectionLost) {
                        setState { copy(nextSpeakerProvider = null, engineConnectionLost = true) }
                    }
                    delay(1000)
                }
            }
        }
    }

    private fun dispatchHeldComment(text: String, isInterrupt: Boolean) {
        val held = text.trim()
        if (held.isEmpty()) return
        setState { copy(queuedUserComment = null) }
        viewModelScope.launch {
            try {
                val currentDisc = state.value.discussion ?: return@launch
                val isPriorCompleted = currentDisc.status.isCompleted
                val currentRound = if (isPriorCompleted) {
                    (currentDisc.transcript.maxOfOrNull { it.round } ?: 1) + 1
                } else {
                    currentDisc.transcript.maxOfOrNull { it.round } ?: 1
                }
                val userMsg = com.dialex.model.DebateMessage(
                    seatId = "observer",
                    provider = Provider.CUSTOM,
                    authorDisplayName = "You (Observer)",
                    round = currentRound,
                    content = held,
                    isUserComment = true,
                    timestampMs = System.currentTimeMillis()
                )
                val updatedTranscript = currentDisc.transcript + userMsg
                val snapshottedArts = if (isPriorCompleted) {
                    val prevRound = currentDisc.transcript.maxOfOrNull { it.round } ?: 1
                    (currentDisc.artifacts + currentDisc.resolveAllArtifacts(snapshotRound = prevRound)).distinctBy { it.id }
                } else {
                    currentDisc.artifacts
                }
                val updatedDisc = if (isPriorCompleted) {
                    currentDisc.copy(
                        status = DiscussionStatus.RUNNING,
                        transcript = updatedTranscript,
                        artifacts = snapshottedArts,
                        conclusion = null,
                        deliverable = null,
                        summary = null
                    )
                } else {
                    currentDisc.copy(transcript = updatedTranscript)
                }
                // Immediately show in chat UI feed
                setState { copy(discussion = updatedDisc) }

                if (isInterrupt && currentDisc.status == DiscussionStatus.RUNNING) {
                    // Abort active in-flight turn immediately and restart with comment injected
                    runCatching { stopDiscussionUseCase(discussionId) }
                    delay(100)
                    updateDiscussionUseCase(updatedDisc)
                    resumeDiscussionUseCase(discussionId)
                    startStreamingOrPolling()
                } else if (currentDisc.status == DiscussionStatus.RUNNING) {
                    updateDiscussionUseCase(updatedDisc)
                } else if (currentDisc.status == DiscussionStatus.PAUSED || currentDisc.status.isCompleted || currentDisc.status.isFailed) {
                    updateDiscussionUseCase(updatedDisc)
                    resumeDiscussionUseCase(discussionId)
                    startStreamingOrPolling()
                } else {
                    updateDiscussionUseCase(updatedDisc)
                }
            } catch (e: Exception) {
                com.dialex.logging.AppLogStore.error("DebateRunner", "Failed to send comment: ${e.message}", e)
                sendEffect(ChatEffect.ShowSnackbar("Failed to send comment: ${e.message}"))
            }
        }
    }

    private fun computeTokenWarning(discussion: Discussion) {
        val totalTokens = discussion.transcript.sumOf { (it.tokensIn ?: 0) + (it.tokensOut ?: 0) }
        val ratio = if (tokenBudget > 0) totalTokens.toDouble() / tokenBudget else 0.0
        val percentage = (ratio * 100).roundToInt()

        val level = when {
            tokenBudget == 0 -> TokenWarningLevel.None
            ratio < 0.5 -> TokenWarningLevel.None
            ratio < 0.65 -> TokenWarningLevel.Caution
            ratio < 0.80 -> TokenWarningLevel.Warning
            ratio < 0.95 -> TokenWarningLevel.High
            ratio < 1.0 -> TokenWarningLevel.Critical
            else -> TokenWarningLevel.Exceeded
        }

        val label = if (tokenBudget > 0) {
            "${formatTokenCount(totalTokens)} / ${formatTokenCount(tokenBudget.toLong())} tokens ($percentage%)"
        } else {
            "${formatTokenCount(totalTokens)} tokens"
        }

        val totalSpend = com.dialex.model.PricingTable.calculateDiscussionSpend(
            discussion.transcript,
            discussion.config.agents
        )

        val totalInTokens = discussion.transcript.sumOf { it.tokensIn ?: 0 }
        val totalCachedTokens = discussion.transcript.sumOf { it.tokensCached ?: 0 }
        val cachedPercent = if (totalInTokens > 0) {
            ((totalCachedTokens.toDouble() / totalInTokens) * 100).roundToInt().coerceIn(0, 100)
        } else 0

        setState { 
            copy(
                tokenWarningLevel = level, 
                tokenProgressLabel = label,
                totalSpendUsd = totalSpend,
                cachedTokensPercent = cachedPercent
            ) 
        }
    }

    private fun updateNextSpeaker(discussion: Discussion) {
        if (discussion.status == DiscussionStatus.RUNNING && discussion.config.agents.isNotEmpty()) {
            val nextAgent = discussion.config.agents[discussion.transcript.size % discussion.config.agents.size]
            setState { copy(nextSpeakerProvider = nextAgent.provider) }
        } else {
            setState { copy(nextSpeakerProvider = null) }
        }
    }

    override fun onIntent(intent: ChatIntent) {
        when (intent) {
            is ChatIntent.Pause -> {
                if (state.value.isActionInProgress) return
                setState { copy(isActionInProgress = true, pauseRequested = true) }
                streamJob?.cancel()
                viewModelScope.launch {
                    try {
                        pauseDiscussionUseCase(discussionId)
                        val updated = getDiscussionUseCase(discussionId)
                        applyUpdatedDiscussion(updated)
                    } catch (e: Exception) {
                        setState { copy(isActionInProgress = false) }
                        sendEffect(ChatEffect.ShowSnackbar(e.message ?: "Failed to pause debate"))
                    }
                }
            }
            is ChatIntent.HardStop -> {
                if (state.value.isActionInProgress) return
                setState { copy(isActionInProgress = true) }
                streamJob?.cancel()
                viewModelScope.launch {
                    try {
                        stopDiscussionUseCase(discussionId)
                        val updated = getDiscussionUseCase(discussionId)
                        applyUpdatedDiscussion(updated)
                    } catch (e: Exception) {
                        setState { copy(isActionInProgress = false) }
                        sendEffect(ChatEffect.ShowSnackbar(e.message ?: "Failed to stop debate"))
                    }
                }
            }
            is ChatIntent.Resume -> {
                if (state.value.isActionInProgress) return
                if (state.value.invalidFolders.isNotEmpty()) {
                    sendEffect(ChatEffect.ShowSnackbar("Cannot resume: A workspace folder was removed or is invalid."))
                    return
                }
                val currentDisc = state.value.discussion ?: return
                if (currentDisc.status == DiscussionStatus.RUNNING) return
                val isResumingFromDone = currentDisc.status.isCompleted
                val lastAgentRound = currentDisc.transcript.filterNot { it.isUserComment || it.isError }.maxOfOrNull { it.round } ?: 1
                val transcriptWithoutTrailingErrors = currentDisc.transcript.dropLastWhile { it.isError }
                val sanitizedTranscript = TranscriptMerger.sanitizeTranscript(transcriptWithoutTrailingErrors, currentDisc.config)
                val targetMaxRound = sanitizedTranscript.maxOfOrNull { it.round } ?: lastAgentRound
                val updatedConfig = if (currentDisc.config.roundMode == com.dialex.model.RoundMode.FIXED) {
                    if (targetMaxRound >= currentDisc.config.maxRounds) {
                        currentDisc.config.copy(maxRounds = targetMaxRound + 2)
                    } else {
                        currentDisc.config
                    }
                } else {
                    currentDisc.config
                }
                val runningDisc = if (isResumingFromDone) {
                    val existingArtifacts = currentDisc.resolveAllArtifacts(snapshotRound = lastAgentRound)
                    currentDisc.copy(
                        config = updatedConfig,
                        transcript = sanitizedTranscript,
                        status = DiscussionStatus.RUNNING,
                        artifacts = existingArtifacts
                    )
                } else {
                    currentDisc.copy(
                        config = updatedConfig,
                        transcript = sanitizedTranscript,
                        status = DiscussionStatus.RUNNING
                    )
                }
                setState {
                    copy(
                        discussion = runningDisc,
                        isActionInProgress = true,
                        pauseRequested = false
                    )
                }
                updateNextSpeaker(runningDisc)
                viewModelScope.launch {
                    try {
                        if (runningDisc != null) {
                            updateDiscussionUseCase(runningDisc)
                        }
                        resumeDiscussionUseCase(discussionId)
                        startStreamingOrPolling()
                    } catch (e: Exception) {
                        setState { copy(discussion = currentDisc, isActionInProgress = false) }
                        if (currentDisc != null) updateNextSpeaker(currentDisc)
                        val msg = e.message ?: "Failed to resume discussion"
                        com.dialex.logging.AppLogStore.error("DebateRunner", "Failed to resume: $msg", e)
                        sendEffect(ChatEffect.ShowSnackbar(msg))
                    }
                }
            }
            is ChatIntent.RestartDebate -> {
                if (state.value.isActionInProgress) return
                if (state.value.invalidFolders.isNotEmpty()) {
                    sendEffect(ChatEffect.ShowSnackbar("Cannot restart: A workspace folder was removed or is invalid."))
                    return
                }
                val currentDisc = state.value.discussion ?: return
                val currentMaxRound = currentDisc.transcript.filterNot { it.isUserComment || it.isError }.maxOfOrNull { it.round } ?: 1
                val sanitizedTranscript = TranscriptMerger.sanitizeTranscript(currentDisc.transcript, currentDisc.config)
                val updatedConfig = if (intent.unlimited) {
                    currentDisc.config.copy(roundMode = com.dialex.model.RoundMode.UNLIMITED)
                } else {
                    val addRounds = if (intent.additionalRounds > 0) intent.additionalRounds else 3
                    val newMax = if (currentDisc.config.roundMode == com.dialex.model.RoundMode.FIXED) {
                        maxOf(currentDisc.config.maxRounds, currentMaxRound) + addRounds
                    } else {
                        currentMaxRound + addRounds
                    }
                    currentDisc.config.copy(roundMode = com.dialex.model.RoundMode.FIXED, maxRounds = newMax)
                }
                val existingArtifacts = currentDisc.resolveAllArtifacts(snapshotRound = currentMaxRound)
                val restartingDisc = currentDisc.copy(
                    config = updatedConfig,
                    transcript = sanitizedTranscript,
                    status = DiscussionStatus.RUNNING,
                    artifacts = existingArtifacts
                )
                setState {
                    copy(
                        discussion = restartingDisc,
                        isActionInProgress = true,
                        pauseRequested = false
                    )
                }
                updateNextSpeaker(restartingDisc)
                viewModelScope.launch {
                    try {
                        updateDiscussionUseCase(restartingDisc)
                        resumeDiscussionUseCase(discussionId)
                        startStreamingOrPolling()
                    } catch (e: Exception) {
                        setState { copy(discussion = currentDisc, isActionInProgress = false) }
                        updateNextSpeaker(currentDisc)
                        val msg = e.message ?: "Failed to continue debate"
                        com.dialex.logging.AppLogStore.error("DebateRunner", "Failed to restart: $msg", e)
                        sendEffect(ChatEffect.ShowSnackbar(msg))
                    }
                }
            }
            is ChatIntent.SendUserComment -> {
                val text = intent.text.trim()
                if (text.isEmpty()) return
                if (state.value.invalidFolders.isNotEmpty()) {
                    sendEffect(ChatEffect.ShowSnackbar("Cannot send comment: A workspace folder was removed or is invalid."))
                    return
                }
                val currentDisc = state.value.discussion ?: return
                if (currentDisc.mode == com.dialex.domain.model.DiscussionMode.SOCRATIC_INTERVIEW) {
                    handleSocraticUserTurn(text, currentDisc)
                    return
                }
                if (currentDisc.status == DiscussionStatus.RUNNING) {
                    setState { copy(queuedUserComment = text) }
                } else if (currentDisc.status.isCompleted || currentDisc.status == DiscussionStatus.PAUSED || currentDisc.status.isFailed) {
                    val lastAgentRound = currentDisc.transcript.filterNot { it.isUserComment || it.isError }.maxOfOrNull { it.round } ?: 0
                    val nextRound = if (currentDisc.status.isCompleted || lastAgentRound == 0) {
                        lastAgentRound + 1
                    } else {
                        val agentTurnsInLastRound = currentDisc.transcript.count { !it.isUserComment && !it.isError && it.round == lastAgentRound }
                        if (agentTurnsInLastRound >= currentDisc.config.agents.size.coerceAtLeast(1)) {
                            lastAgentRound + 1
                        } else {
                            lastAgentRound
                        }
                    }

                    val userMsg = com.dialex.model.DebateMessage(
                        seatId = "observer",
                        provider = Provider.CUSTOM,
                        authorDisplayName = "You (Observer)",
                        round = nextRound,
                        content = text,
                        isUserComment = true,
                        timestampMs = System.currentTimeMillis()
                    )
                    val updatedTranscript = TranscriptMerger.sanitizeTranscript(currentDisc.transcript + userMsg, currentDisc.config)
                    val existingArtifacts = if (lastAgentRound > 0) {
                        currentDisc.resolveAllArtifacts(snapshotRound = lastAgentRound)
                    } else {
                        currentDisc.artifacts
                    }
                    val updatedConfig = if (currentDisc.config.roundMode == com.dialex.model.RoundMode.FIXED) {
                        val newMax = maxOf(currentDisc.config.maxRounds, nextRound) + 2
                        currentDisc.config.copy(maxRounds = newMax)
                    } else {
                        currentDisc.config
                    }
                    val updatedDisc = currentDisc.copy(
                        config = updatedConfig,
                        transcript = updatedTranscript,
                        status = DiscussionStatus.RUNNING,
                        artifacts = existingArtifacts,
                        conclusion = null,
                        deliverable = null,
                        summary = null
                    )
                    setState { copy(discussion = updatedDisc, pauseRequested = false) }
                    updateNextSpeaker(updatedDisc)
                    viewModelScope.launch {
                        runCatching {
                            updateDiscussionUseCase(updatedDisc)
                            resumeDiscussionUseCase(updatedDisc.id)
                            startStreamingOrPolling()
                        }.onFailure { err ->
                            val msg = err.message ?: "unknown error"
                            com.dialex.logging.AppLogStore.error("DebateRunner", "Failed to resume discussion with user comment: $msg", err)
                            sendEffect(ChatEffect.ShowSnackbar("Failed to resume discussion: $msg"))
                        }
                    }
                } else {
                    val userMsg = com.dialex.model.DebateMessage(
                        seatId = "observer",
                        provider = Provider.CUSTOM,
                        authorDisplayName = "You (Observer)",
                        round = (currentDisc.transcript.maxOfOrNull { it.round } ?: 1),
                        content = text,
                        isUserComment = true,
                        timestampMs = System.currentTimeMillis()
                    )
                    val updatedTranscript = TranscriptMerger.sanitizeTranscript(currentDisc.transcript + userMsg, currentDisc.config)
                    val updatedDisc = currentDisc.copy(transcript = updatedTranscript)
                    setState { copy(discussion = updatedDisc) }
                    viewModelScope.launch {
                        runCatching {
                            updateDiscussionUseCase(updatedDisc)
                        }
                    }
                }
            }
            is ChatIntent.InterruptAndSendHeldComment -> {
                val held = state.value.queuedUserComment ?: return
                dispatchHeldComment(held, isInterrupt = true)
            }
            is ChatIntent.CancelHeldComment -> {
                setState { copy(queuedUserComment = null) }
            }
            is ChatIntent.SaveArtifacts -> {
                val currentDisc = state.value.discussion ?: return
                val merged = (currentDisc.artifacts + intent.artifacts).distinctBy { it.id }
                val updatedDisc = currentDisc.copy(artifacts = merged)
                setState { copy(discussion = updatedDisc) }
                viewModelScope.launch {
                    runCatching {
                        updateDiscussionUseCase(updatedDisc)
                    }
                }
            }
            is ChatIntent.DeleteArtifact -> {
                val currentDisc = state.value.discussion ?: return
                val updatedArtifacts = currentDisc.artifacts.filterNot { it.id == intent.artifactId }
                val updatedDisc = currentDisc.copy(artifacts = updatedArtifacts)
                setState { copy(discussion = updatedDisc) }
                viewModelScope.launch {
                    runCatching {
                        updateDiscussionUseCase(updatedDisc)
                        sendEffect(ChatEffect.ShowSnackbar("Artifact deleted"))
                    }
                }
            }
            is ChatIntent.DismissArtifactBanner -> {
                val currentDisc = state.value.discussion ?: return
                if (intent.artifactId in currentDisc.dismissedArtifactIds) return
                val updatedDisc = currentDisc.copy(
                    dismissedArtifactIds = currentDisc.dismissedArtifactIds + intent.artifactId
                )
                setState { copy(discussion = updatedDisc) }
                viewModelScope.launch {
                    runCatching {
                        updateDiscussionUseCase(updatedDisc)
                    }
                }
            }
            is ChatIntent.GenerateAlternativeDeliverable -> {
                val currentDisc = state.value.discussion ?: return
                if (state.value.generatingDeliverableFormat != null) return
                val formatLabel = intent.format.name.lowercase().replace('_', ' ')
                    .split(' ').joinToString(" ") { it.replaceFirstChar { c -> c.uppercase() } }
                val deliverableName = "$formatLabel Deliverable"
                viewModelScope.launch {
                    setState { copy(generatingDeliverableFormat = intent.format) }
                    try {
                        sendEffect(ChatEffect.ShowSnackbar("Generating $deliverableName…"))
                        val content = generateDeliverableFormatUseCase(
                            discussionId = currentDisc.id,
                            format = intent.format
                        )
                        val now = System.currentTimeMillis()
                        val currentRound = currentDisc.transcript.maxOfOrNull { it.round } ?: 1
                        val artifact = com.dialex.model.DiscussionArtifact(
                            id = "${intent.format.name.lowercase()}_$now",
                            name = deliverableName,
                            type = "Deliverable",
                            format = "md",
                            content = content,
                            timestamp = com.dialex.util.formatMessageTimestamp(now),
                            timestampMs = now,
                            sizeBytes = content.encodeToByteArray().size.toLong(),
                            round = currentRound
                        )
                        val updatedDisc = currentDisc.copy(
                            deliverable = content,
                            config = currentDisc.config.copy(
                                deliverable = currentDisc.config.deliverable.copy(format = intent.format)
                            ),
                            artifacts = currentDisc.artifacts.filter { it.name != deliverableName } + artifact
                        )
                        setState { copy(discussion = updatedDisc, generatingDeliverableFormat = null) }
                        updateDiscussionUseCase(updatedDisc)
                        sendEffect(ChatEffect.ShowSnackbar("$deliverableName generated"))
                    } catch (e: Exception) {
                        setState { copy(generatingDeliverableFormat = null) }
                        sendEffect(ChatEffect.ShowSnackbar("Failed to generate $deliverableName: ${e.message ?: "network error"}"))
                    }
                }
            }
            is ChatIntent.ToggleArtifactsModal -> {
                setState { copy(artifactsModalOpen = !artifactsModalOpen) }
            }
            is ChatIntent.EditSetup -> {
                sendEffect(ChatEffect.NavigateToSetup(discussionId))
            }
            is ChatIntent.GenerateHandoffPrompt -> {
                setState { copy(handoffState = HandoffState.Loading) }
                viewModelScope.launch {
                    try {
                        generateHandoffPromptUseCase(discussionId)
                        setState { copy(handoffState = HandoffState.Idle) }
                        sendEffect(ChatEffect.ShowSnackbar("Handoff prompt copied — also shown below"))
                    } catch (e: Exception) {
                        setState { copy(handoffState = HandoffState.Failed("Failed to generate prompt")) }
                    }
                }
            }
            is ChatIntent.ExportMarkdown -> {
                val discussion = state.value.discussion ?: return
                sendEffect(ChatEffect.ExportMarkdown(discussion.toMarkdown(), discussion.exportFileName()))
            }
            is ChatIntent.ShowUsageModal -> {
                viewModelScope.launch {
                    val discussion = state.value.discussion ?: return@launch
                    val breakdown = discussion.config.agents.map { agent ->
                        val agentMessages = discussion.transcript.filter { it.isFrom(agent) }
                        val inT = agentMessages.sumOf { (it.tokensIn ?: 0).toLong() }
                        val outT = agentMessages.sumOf { (it.tokensOut ?: 0).toLong() }
                        val cachedT = agentMessages.sumOf { (it.tokensCached ?: 0).toLong() }
                        val cost = com.dialex.model.PricingTable.calculateSpend(
                            model = agent.model,
                            tokensIn = inT,
                            tokensOut = outT,
                            tokensCached = cachedT,
                            provider = agent.provider
                        )
                        AgentUsage(
                            agentName = agent.label(),
                            provider = agent.provider,
                            tokensIn = inT,
                            tokensOut = outT,
                            estimatedCostUsd = cost
                        )
                    }.toImmutableList()
                    setState { copy(showUsageModal = true, usageBreakdown = breakdown) }
                }
            }
            is ChatIntent.DismissUsageModal -> {
                setState { copy(showUsageModal = false) }
            }
            is ChatIntent.DuplicateConfig -> {
                viewModelScope.launch {
                    try {
                        val dup = duplicateDiscussionUseCase(discussionId)
                        sendEffect(ChatEffect.NavigateToSetup(dup.id))
                    } catch (e: Exception) {
                        sendEffect(ChatEffect.ShowSnackbar("Failed to duplicate configuration"))
                    }
                }
            }
            is ChatIntent.ScrollToBottom -> {
                sendEffect(ChatEffect.ScrollToBottom)
            }
            is ChatIntent.ApproveCommand -> {
                // Reserved for interactive tool approval
            }
            is ChatIntent.RejectCommand -> {
                // Reserved for interactive tool rejection
            }
            is ChatIntent.RenameDiscussion -> {
                viewModelScope.launch {
                    val current = state.value.discussion ?: return@launch
                    val trimmed = intent.newName.trim()
                    if (trimmed.isNotBlank() && trimmed != current.name) {
                        try {
                            val updated = updateDiscussionUseCase(current.copy(name = trimmed))
                            setState { copy(discussion = updated) }
                        } catch (e: Exception) {
                            sendEffect(ChatEffect.ShowSnackbar("Failed to rename discussion"))
                        }
                    }
                }
            }
            is ChatIntent.GenerateSummaryTitle -> {
                viewModelScope.launch {
                    val current = state.value.discussion ?: return@launch
                    try {
                        val newTitle = generateDiscussionTitleUseCase(current.id)
                        if (newTitle.isNotBlank()) {
                            val updated = current.copy(name = newTitle)
                            setState { copy(discussion = updated) }
                            sendEffect(ChatEffect.ShowSnackbar("Summary title: $newTitle"))
                        }
                    } catch (e: Exception) {
                        sendEffect(ChatEffect.ShowSnackbar("Failed to generate summary title"))
                    }
                }
            }
            is ChatIntent.RemoveInvalidFolder -> {
                val current = state.value.discussion ?: return
                val updatedFolders = current.attachedFolders.filter { it.path != intent.path }
                val updated = current.copy(attachedFolders = updatedFolders)
                setState { copy(discussion = updated) }
                viewModelScope.launch {
                    try {
                        updateDiscussionUseCase(updated)
                        validateWorkspaceFolders(updated)
                    } catch (e: Exception) {}
                }
            }
            is ChatIntent.RetryFolderValidation -> {
                val current = state.value.discussion ?: return
                validateWorkspaceFolders(current)
            }
            is ChatIntent.ToggleTensionDrawer -> {
                setState { copy(isTensionDrawerOpen = !isTensionDrawerOpen) }
            }
            is ChatIntent.SetTensionDrawerOpen -> {
                setState { copy(isTensionDrawerOpen = intent.open) }
            }
            is ChatIntent.SetTensionFilter -> {
                setState { copy(tensionFilter = intent.filter) }
            }
            is ChatIntent.ToggleEvidenceDrawer -> {
                setState {
                    copy(
                        isEvidenceDrawerOpen = !isEvidenceDrawerOpen,
                        selectedEvidenceRound = if (!isEvidenceDrawerOpen) intent.round else null
                    )
                }
            }
            is ChatIntent.SetEvidenceDrawerOpen -> {
                setState {
                    copy(
                        isEvidenceDrawerOpen = intent.open,
                        selectedEvidenceRound = if (intent.open) intent.round else null
                    )
                }
            }
            is ChatIntent.ToggleCredenceDrawer -> {
                setState { copy(isCredenceDrawerOpen = !isCredenceDrawerOpen) }
            }
            is ChatIntent.SetCredenceDrawerOpen -> {
                setState { copy(isCredenceDrawerOpen = intent.open) }
            }
            is ChatIntent.SelectCredenceRound -> {
                setState { copy(selectedCredenceRound = intent.round) }
            }
            is ChatIntent.RecalculateCredence -> {
                recalculateCredence()
            }
            is ChatIntent.GenerateSocraticDigest -> {
                if (state.value.isGeneratingDigest) return
                val currentDisc = state.value.discussion ?: return
                setState { copy(isGeneratingDigest = true) }
                viewModelScope.launch {
                    socraticSessionDelegate.generateDigest(discussionId, currentDisc)
                        .onSuccess { (updated, digest) ->
                            setState {
                                copy(
                                    discussion = updated,
                                    socraticDigest = digest,
                                    isGeneratingDigest = false
                                )
                            }
                            sendEffect(ChatEffect.ScrollToBottom)
                            sendEffect(ChatEffect.ShowSnackbar("Socratic Epistemic Digest generated & synced to Knowledge Graph!"))
                        }
                        .onFailure { e ->
                            setState { copy(isGeneratingDigest = false) }
                            val msg = e.message ?: "Failed to generate digest"
                            sendEffect(ChatEffect.ShowSnackbar(msg))
                        }
                }
            }
            is ChatIntent.ElevateSocraticToCouncil -> {
                if (state.value.isElevatingToCouncil) return
                val digest = state.value.socraticDigest ?: return
                val targetProject = intent.targetProjectId ?: (state.value.discussion?.projectId?.ifBlank { "default" } ?: "default")
                setState { copy(isElevatingToCouncil = true) }
                viewModelScope.launch {
                    socraticSessionDelegate.elevateToCouncil(discussionId, digest, targetProject)
                        .onSuccess { result ->
                            setState { copy(isElevatingToCouncil = false) }
                            sendEffect(ChatEffect.ElevateSuccess(result.newDiscussionId))
                            sendEffect(ChatEffect.NavigateToSetup(result.newDiscussionId))
                        }
                        .onFailure { e ->
                            setState { copy(isElevatingToCouncil = false) }
                            val msg = e.message ?: "Failed to elevate to council"
                            sendEffect(ChatEffect.ShowSnackbar(msg))
                        }
                }
            }
        }
    }

    private fun handleSocraticUserTurn(statement: String, currentDisc: Discussion) {
        if (state.value.isSocraticProcessing) return
        setState {
            copy(
                isSocraticProcessing = true,
                nextSpeakerProvider = currentDisc.config.primary.provider
            )
        }
        sendEffect(ChatEffect.ScrollToBottom)

        viewModelScope.launch {
            socraticSessionDelegate.handleUserTurn(
                discussionId = discussionId,
                statement = statement,
                currentDisc = currentDisc,
                currentStage = state.value.socraticStage
            ).onSuccess { res ->
                setState {
                    copy(
                        discussion = res.updatedDiscussion,
                        isSocraticProcessing = false,
                        nextSpeakerProvider = null,
                        activeProbe = res.probeQuestion,
                        socraticStage = res.newStage
                    )
                }
                sendEffect(ChatEffect.ScrollToBottom)
            }.onFailure { e ->
                setState { copy(isSocraticProcessing = false, nextSpeakerProvider = null) }
                val msg = e.message ?: "Failed to conduct Socratic turn"
                sendEffect(ChatEffect.ShowSnackbar(msg))
            }
        }
    }

    private fun validateWorkspaceFolders(discussion: Discussion) {
        val invalid = com.dialex.util.validateFolders(discussion.attachedFolders)
        setState { copy(invalidFolders = invalid.toImmutableList()) }
        if (invalid.isNotEmpty() && discussion.status == DiscussionStatus.RUNNING) {
            viewModelScope.launch {
                try {
                    pauseDiscussionUseCase(discussionId)
                } catch (e: Exception) {}
            }
        }
    }

    private fun recalculateCredence() {
        if (state.value.isRecalculatingCredence) return
        val useCase = recalculateCredenceUseCase ?: return
        viewModelScope.launch {
            setState { copy(isRecalculatingCredence = true) }
            try {
                val updatedLedger = useCase(discussionId).getOrThrow()
                setState {
                    copy(
                        credenceLedger = updatedLedger,
                        isRecalculatingCredence = false,
                        discussion = discussion?.copy(credenceLedger = updatedLedger)
                    )
                }
            } catch (e: Exception) {
                setState { copy(isRecalculatingCredence = false) }
                com.dialex.logging.AppLogStore.error("ChatViewModel", "Recalculate credence failed: ${e.message}")
            }
        }
    }

    override fun onCleared() {
        streamJob?.cancel()
        super.onCleared()
    }
}
