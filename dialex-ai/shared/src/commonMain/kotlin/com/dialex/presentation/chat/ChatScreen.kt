@file:Suppress("DEPRECATION")

package com.dialex.presentation.chat

import com.dialex.presentation.chat.components.*

import com.dialex.domain.model.DiscussionMode
import com.dialex.domain.model.SocraticStance
import com.dialex.domain.model.SocraticStage
import androidx.compose.animation.core.*
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import kotlinx.coroutines.Job
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.hoverable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsHoveredAsState
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.foundation.layout.*
import androidx.compose.ui.input.key.*
import com.dialex.util.formatMessageTimestamp
import com.dialex.util.formatTokenCount
import com.dialex.service.launchCliLoginTerminal
import com.dialex.service.SupportedCliTools
import com.dialex.service.CliToolDescriptor
import com.dialex.ui.cliauth.CliAuthDialog
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.horizontalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.*
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.*
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.window.Dialog
import com.dialex.ui.GradientButton
import com.dialex.ui.ThemedTooltipBox
import com.dialex.export.exportMemoFileName
import com.dialex.export.resolveExportFileName
import com.dialex.export.toDeliverableMarkdown
import com.dialex.export.toExecutiveMemorandumHtml
import com.dialex.export.toMarkdown
import com.dialex.export.toSummaryMarkdown
import com.dialex.domain.model.ConsensusResult
import com.dialex.model.DeliverableFormat
import com.dialex.model.DiscussionArtifact
import com.dialex.util.writeTextFile
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.zIndex
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.Canvas
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.model.Agent
import com.dialex.model.DebateConfig
import com.dialex.model.DebateMessage
import com.dialex.model.Discussion
import com.dialex.model.DiscussionStatus
import com.dialex.model.Provider
import com.dialex.model.RoundMode
import com.dialex.model.brandName
import com.dialex.model.label
import com.dialex.presentation.setup.TokenWarningLevel
import com.dialex.domain.model.TensionStatus
import com.dialex.theme.CcPalette
import com.dialex.theme.LocalCcColors
import com.dialex.ui.MarkdownText
import com.dialex.presentation.workspace.WorkspaceHeader
import com.dialex.service.isPlatformCliSupported
import com.dialex.service.checkPlatformCliLogins
import kolt.utils.state.AsyncState
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private data class SearchMatchOccurrence(
    val transcriptIndex: Int,
    val occurrenceIndexInMessage: Int,
    val matchCharOffset: Int = 0
)

private val shownArtifactToastedDiscussions = mutableSetOf<String>()

internal val LocalToast = staticCompositionLocalOf<(String) -> Unit> { {} }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    state: ChatState,
    onIntent: (ChatIntent) -> Unit,
    onBack: (() -> Unit)? = null,
    projectName: String? = null,
    onEditSetup: (() -> Unit)? = null,
    onExportMarkdown: (() -> Unit)? = null,
    onExportMemo: (() -> Unit)? = null,
    onOpenSettings: (() -> Unit)? = null,
    onOpenTokenSettings: (() -> Unit)? = null,
    isCompact: Boolean = false,
    externalArtifactsPaneOpen: Boolean? = null,
    onExternalToggleArtifacts: (() -> Unit)? = null,
    externalSummaryDialogOpen: Boolean? = null,
    onExternalToggleSummary: (() -> Unit)? = null,
    externalSearchQuery: String? = null,
    onExternalSearchQueryChange: ((String) -> Unit)? = null,
    externalSearchActive: Boolean? = null,
    onExternalToggleSearch: ((Boolean) -> Unit)? = null,
    externalSearchNextTrigger: Int = 0,
    externalSearchPrevTrigger: Int = 0,
    onSearchMatchesChanged: ((count: Int, currentIndex: Int) -> Unit)? = null,
    snackbarMessage: String? = null,
    onClearSnackbar: (() -> Unit)? = null,
    evaluateConsensus: (suspend (DebateConfig, List<DebateMessage>) -> ConsensusResult?)? = null,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    val listState = rememberLazyListState()
    val snackbarHostState = remember { SnackbarHostState() }
    val toastScope = rememberCoroutineScope()
    var activeAuthCliTool by remember { mutableStateOf<CliToolDescriptor?>(null) }
    var activeCliLogins by remember {
        mutableStateOf(if (isPlatformCliSupported()) checkPlatformCliLogins() else emptyMap())
    }

    LaunchedEffect(Unit) {
        if (isPlatformCliSupported()) {
            activeCliLogins = checkPlatformCliLogins()
        }
    }

    LaunchedEffect(snackbarMessage) {
        if (snackbarMessage != null) {
            snackbarHostState.showSnackbar(snackbarMessage, duration = SnackbarDuration.Short)
            onClearSnackbar?.invoke()
        }
    }

    val discussion = state.discussion

    if (discussion == null) {
        Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            if (state.loadAsync is AsyncState.Loading) {
                CircularProgressIndicator(color = cc.textPrimary)
            } else {
                Text("Error loading discussion", color = MaterialTheme.colorScheme.error)
            }
        }
        return
    }

    val nextSpeakerProvider = state.nextSpeakerProvider
    val handoff = state.handoffState
    val nextSpeakerAgent = discussion.config.agents.find { it.provider == nextSpeakerProvider }

    var inputPrompt by remember { mutableStateOf("") }
    var selectedAgentMode by remember { mutableStateOf("All Agents") }
    var agentModeDropdownOpen by remember { mutableStateOf(false) }
    var isDebateErrorDialogOpen by remember { mutableStateOf(false) }
    var isContinueDebateDialogOpen by remember { mutableStateOf(false) }

    // Chat search state management
    var internalSearchQuery by remember { mutableStateOf("") }
    var internalSearchActive by remember { mutableStateOf(false) }
    var currentMatchIndex by remember(discussion.id) { mutableStateOf(0) }

    val effectiveSearchQuery = externalSearchQuery ?: internalSearchQuery
    val effectiveSearchActive = externalSearchActive ?: internalSearchActive
    val onEffectiveSearchQueryChange: (String) -> Unit = { query ->
        if (onExternalSearchQueryChange != null) onExternalSearchQueryChange(query) else internalSearchQuery = query
    }
    val onEffectiveToggleSearch: (Boolean) -> Unit = { active ->
        if (onExternalToggleSearch != null) onExternalToggleSearch(active) else {
            internalSearchActive = active
            if (!active) internalSearchQuery = ""
        }
    }

    // Occurrences of search query across all transcript turns
    val searchMatches = remember(discussion.transcript, effectiveSearchQuery) {
        val q = effectiveSearchQuery.trim()
        if (q.isBlank()) {
            emptyList<SearchMatchOccurrence>()
        } else {
            val matches = mutableListOf<SearchMatchOccurrence>()
            discussion.transcript.forEachIndexed { idx, msg ->
                val agentLabel = discussion.labelFor(msg)
                val agentConfig = discussion.config.agents.find { it.id == msg.seatId } ?: discussion.config.agents.find { it.provider == msg.agentId }
                val modelName = agentConfig?.model ?: ""
                val personaRole = agentConfig?.role ?: ""

                var countInContent = 0
                var sIdx = msg.content.indexOf(q, 0, ignoreCase = true)
                while (sIdx != -1) {
                    matches.add(SearchMatchOccurrence(transcriptIndex = idx, occurrenceIndexInMessage = countInContent, matchCharOffset = sIdx))
                    countInContent++
                    sIdx = msg.content.indexOf(q, sIdx + q.length.coerceAtLeast(1), ignoreCase = true)
                }

                if (countInContent == 0) {
                    if (agentLabel.contains(q, ignoreCase = true) ||
                        modelName.contains(q, ignoreCase = true) ||
                        personaRole.contains(q, ignoreCase = true) ||
                        msg.agentId.name.contains(q, ignoreCase = true) ||
                        (msg.isUserComment && "Human User Comment Observer".contains(q, ignoreCase = true))) {
                        matches.add(SearchMatchOccurrence(transcriptIndex = idx, occurrenceIndexInMessage = 0, matchCharOffset = 0))
                    }
                }
            }
            matches
        }
    }

    // Synchronize match count and current index with parent/header
    LaunchedEffect(searchMatches.size, currentMatchIndex) {
        if (searchMatches.isNotEmpty() && currentMatchIndex >= searchMatches.size) {
            currentMatchIndex = 0
        }
        onSearchMatchesChanged?.invoke(
            searchMatches.size,
            if (searchMatches.isEmpty()) 0 else currentMatchIndex
        )
    }

    // Next trigger from header
    LaunchedEffect(externalSearchNextTrigger) {
        if (externalSearchNextTrigger > 0 && searchMatches.isNotEmpty()) {
            currentMatchIndex = (currentMatchIndex + 1) % searchMatches.size
        }
    }

    // Prev trigger from header
    LaunchedEffect(externalSearchPrevTrigger) {
        if (externalSearchPrevTrigger > 0 && searchMatches.isNotEmpty()) {
            currentMatchIndex = (currentMatchIndex - 1 + searchMatches.size) % searchMatches.size
        }
    }

    // Reset currentMatchIndex when search query changes
    LaunchedEffect(effectiveSearchQuery) {
        currentMatchIndex = 0
    }

    // Unified scroll to active match whenever currentMatchIndex or effectiveSearchQuery changes
    LaunchedEffect(currentMatchIndex, effectiveSearchQuery) {
        if (effectiveSearchQuery.isNotBlank() && searchMatches.isNotEmpty() && currentMatchIndex in searchMatches.indices) {
            val match = searchMatches[currentMatchIndex]
            val headerOffset = 1 + (if (state.invalidFolders.isNotEmpty()) 1 else 0)
            val targetItemIndex = (headerOffset + match.transcriptIndex).coerceAtLeast(0)
            val msg = discussion.transcript.getOrNull(match.transcriptIndex)
            val scrollOffset = if (msg != null) calculateMatchScrollOffset(msg.content, match.matchCharOffset) else 0
            listState.animateScrollToItem(targetItemIndex, scrollOffset = scrollOffset)
        }
    }

    val onNextSearchMatch: () -> Unit = {
        if (searchMatches.isNotEmpty()) {
            currentMatchIndex = (currentMatchIndex + 1) % searchMatches.size
        }
    }

    val onPrevSearchMatch: () -> Unit = {
        if (searchMatches.isNotEmpty()) {
            currentMatchIndex = (currentMatchIndex - 1 + searchMatches.size) % searchMatches.size
        }
    }

    val hasDeliverable = (discussion.deliverable != null || discussion.conclusion != null || discussion.summary != null || state.generatingDeliverableFormat != null)

    val isSearchEngaged = effectiveSearchActive || effectiveSearchQuery.isNotBlank()

    // Scroll all the way to bottom on initial load and keep following live turns (unless actively searching)
    var hasScrolledToBottomInitially by remember(discussion.id) { mutableStateOf(false) }
    LaunchedEffect(discussion.id, discussion.transcript.size, listState.layoutInfo.totalItemsCount, nextSpeakerProvider, handoff, discussion.handoffPrompt, discussion.conclusion) {
        val count = listState.layoutInfo.totalItemsCount
        if (count > 0 && !isSearchEngaged) {
            if (!hasScrolledToBottomInitially && (discussion.transcript.isNotEmpty() || discussion.conclusion != null)) {
                listState.scrollToItem(count - 1)
                hasScrolledToBottomInitially = true
            } else if (hasScrolledToBottomInitially || discussion.status == DiscussionStatus.RUNNING) {
                listState.animateScrollToItem(count - 1)
            }
        }
    }

    val chatTypographyState = LocalChatTypographySettings.current
    val typographySettings by chatTypographyState
    val chatDisplayState = LocalChatDisplaySettings.current
    val chatDisplaySettings by chatDisplayState

    var internalArtifactsPaneOpen by remember { mutableStateOf(false) }
    val isArtifactsPaneOpen = externalArtifactsPaneOpen ?: internalArtifactsPaneOpen
    val totalArtifactsList = remember(discussion) { discussion.resolveAllArtifacts() }
    var lastSeenArtifactsCount by remember(discussion.id) { mutableStateOf(0) }
    val hasUnreadArtifacts = totalArtifactsList.size > lastSeenArtifactsCount && !isArtifactsPaneOpen

    val toggleArtifactsPane: () -> Unit = {
        val willOpen = !isArtifactsPaneOpen
        if (onExternalToggleArtifacts != null) onExternalToggleArtifacts() else internalArtifactsPaneOpen = willOpen
        if (willOpen) {
            lastSeenArtifactsCount = totalArtifactsList.size
        }
    }

    var internalSummaryDialogOpen by remember { mutableStateOf(false) }
    val isSummaryDialogOpen = externalSummaryDialogOpen ?: internalSummaryDialogOpen
    val toggleSummaryDialog: () -> Unit = {
        if (onExternalToggleSummary != null) onExternalToggleSummary() else internalSummaryDialogOpen = !internalSummaryDialogOpen
    }
    var hasAutoSavedForThisRun by remember(discussion.id) { mutableStateOf(false) }

    val systemLogs by com.dialex.logging.AppLogStore.logs.collectAsState()
    val latestAppError = remember(systemLogs) {
        systemLogs.findLast { it.level == com.dialex.logging.AppLogLevel.ERROR }?.message
    }

    // If discussion already has artifacts loaded from past runs, mark as toasted so toast never re-fires
    LaunchedEffect(discussion.id) {
        if (discussion.artifacts.isNotEmpty()) {
            shownArtifactToastedDiscussions.add(discussion.id)
        }
    }

    // Auto-save artifacts when discussion completes
    LaunchedEffect(discussion.status, discussion.id) {
        if (discussion.status.isCompleted && !hasAutoSavedForThisRun) {
            val out = discussion.config.output
            if (out.saveArtifacts) {
                hasAutoSavedForThisRun = true
                val newArtifacts = mutableListOf<DiscussionArtifact>()
                var savedDiskCount = 0
                val currentMaxRound = discussion.transcript.maxOfOrNull { it.round } ?: 1
                val lastTurn = discussion.transcript.findLast { it.round == currentMaxRound } ?: discussion.transcript.lastOrNull()
                val now = lastTurn?.timestampMs?.takeIf { it > 0L } ?: System.currentTimeMillis()
                val timeLabel = formatMessageTimestamp(now)

                if (out.autoSaveSummary && (discussion.summary != null || discussion.conclusion != null)) {
                    val fn = discussion.resolveExportFileName("summary")
                    val content = discussion.toSummaryMarkdown()
                    if (out.outputFolder.isNotBlank() && writeTextFile(out.outputFolder, fn, content, out.appendMode)) {
                        savedDiskCount++
                    }
                    val sumId = "art_sum_r$currentMaxRound"
                    if (discussion.artifacts.none { it.id == sumId || it.name == fn }) {
                        newArtifacts.add(
                            DiscussionArtifact(
                                id = sumId,
                                name = fn,
                                type = "Summary",
                                format = "md",
                                content = content,
                                timestamp = timeLabel,
                                timestampMs = now,
                                sizeBytes = content.encodeToByteArray().size.toLong(),
                                round = currentMaxRound
                            )
                        )
                    }
                }

                if (out.autoSaveFullTranscript && discussion.transcript.isNotEmpty()) {
                    val fn = discussion.resolveExportFileName("transcript")
                    val content = discussion.toMarkdown()
                    if (out.outputFolder.isNotBlank() && writeTextFile(out.outputFolder, fn, content, out.appendMode)) {
                        savedDiskCount++
                    }
                    val traId = "art_tra_r$currentMaxRound"
                    if (discussion.artifacts.none { it.id == traId || it.name == fn }) {
                        newArtifacts.add(
                            DiscussionArtifact(
                                id = traId,
                                name = fn,
                                type = "Transcript",
                                format = "md",
                                content = content,
                                timestamp = timeLabel,
                                timestampMs = now,
                                sizeBytes = content.encodeToByteArray().size.toLong(),
                                round = currentMaxRound
                            )
                        )
                    }
                }

                if (out.autoSaveDeliverable && (discussion.deliverable != null || discussion.conclusion != null)) {
                    val formatLabel = discussion.config.deliverable.format.name.lowercase().replace('_', ' ')
                        .split(' ').joinToString(" ") { it.replaceFirstChar { c -> c.uppercase() } }
                    val deliverableName = "$formatLabel Deliverable"
                    val fn = discussion.resolveExportFileName("deliverable")
                    val content = discussion.toDeliverableMarkdown()
                    if (out.outputFolder.isNotBlank() && writeTextFile(out.outputFolder, fn, content, out.appendMode)) {
                        savedDiskCount++
                    }
                    val delId = "art_del_r$currentMaxRound"
                    if (discussion.artifacts.none { it.id == delId || it.name == deliverableName }) {
                        newArtifacts.add(
                            DiscussionArtifact(
                                id = delId,
                                name = deliverableName,
                                type = "Deliverable",
                                format = "md",
                                content = content,
                                timestamp = timeLabel,
                                timestampMs = now,
                                sizeBytes = content.encodeToByteArray().size.toLong(),
                                round = currentMaxRound
                            )
                        )
                    }
                }

                if (newArtifacts.isNotEmpty()) {
                    onIntent(ChatIntent.SaveArtifacts(newArtifacts))
                }

                val msg = if (savedDiskCount > 0) {
                    "Auto-saved $savedDiskCount debate file(s) to ${out.outputFolder} & engine repository"
                } else if (newArtifacts.isNotEmpty()) {
                    "Auto-saved ${newArtifacts.size} artifact(s) to engine repository"
                } else null

                if (msg != null && shownArtifactToastedDiscussions.add(discussion.id)) {
                    toastScope.launch {
                        snackbarHostState.showSnackbar(msg, duration = SnackbarDuration.Short)
                    }
                }
            }
        }
    }

    CompositionLocalProvider(
        LocalToast provides { message ->
            toastScope.launch { snackbarHostState.showSnackbar(message, duration = SnackbarDuration.Short) }
        },
    ) {
        BoxWithConstraints(
            modifier.fillMaxSize()
                .background(cc.bg)
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown &&
                        (event.isMetaPressed || event.isCtrlPressed) &&
                        event.isShiftPressed &&
                        event.key == Key.B
                    ) {
                        onIntent(ChatIntent.ToggleCredenceDrawer)
                        true
                    } else if (event.type == KeyEventType.KeyDown &&
                        (event.isMetaPressed || event.isCtrlPressed) &&
                        event.key == Key.F
                    ) {
                        onEffectiveToggleSearch(!effectiveSearchActive)
                        true
                    } else false
                }
        ) {
            val isWideDisplay = maxWidth >= 900.dp && !isCompact

            Column(Modifier.fillMaxSize()) {
                // Header on compact screens (wide-screen header is provided by App.kt)
                if (isCompact) {
                    WorkspaceHeader(
                        projectName = projectName,
                        discussionName = discussion.name.ifBlank { "Dialex Debate" },
                        status = discussion.status,
                        readingSettings = typographySettings,
                        onReadingSettingsChange = { chatTypographyState.value = it },
                        onBack = onBack,
                        onEditSetup = onEditSetup,
                        onExportMarkdown = onExportMarkdown,
                        onExportMemo = onExportMemo,
                        onToggleSummary = toggleSummaryDialog,
                        onOpenArtifacts = toggleArtifactsPane,
                        artifactsCount = totalArtifactsList.size,
                        hasUnreadArtifacts = hasUnreadArtifacts,
                        onRenameDiscussion = { newName -> onIntent(ChatIntent.RenameDiscussion(newName)) },
                        onOpenSettings = onOpenSettings ?: {},
                        searchQuery = effectiveSearchQuery,
                        onSearchQueryChange = onEffectiveSearchQueryChange,
                        isSearchActive = effectiveSearchActive,
                        onToggleSearch = onEffectiveToggleSearch,
                        searchMatchCount = searchMatches.size,
                        currentSearchMatchIndex = currentMatchIndex,
                        onNextSearchMatch = onNextSearchMatch,
                        onPrevSearchMatch = onPrevSearchMatch,
                        openTensionCount = state.tensionPairs.count { it.status == TensionStatus.OPEN || it.status == TensionStatus.EXPLORED },
                        onOpenTensionDrawer = { onIntent(ChatIntent.SetTensionDrawerOpen(true)) },
                        retrievedEvidenceCount = state.retrievedEvidence.sumOf { it.items.size },
                        onOpenEvidenceDrawer = { onIntent(ChatIntent.SetEvidenceDrawerOpen(true, null)) },
                        credenceLedger = state.credenceLedger,
                        onOpenCredenceDrawer = { onIntent(ChatIntent.SetCredenceDrawerOpen(true)) }
                    )
                }

                // ── Main Aerated Transcript Area & Optional Summary Right Panel ───
                Row(Modifier.weight(1f).fillMaxWidth()) {
                    // Discussion Space (Transcript + Floating Bottom Input Bar + FAB + Snackbar)
                    BoxWithConstraints(Modifier.weight(1f).fillMaxHeight()) {
                        // Fluid-responsive reading measure: expands gracefully with screen width, clamped at 880dp
                        val maxContentWidth = when {
                            maxWidth >= 1200.dp -> 880.dp
                            maxWidth >= 960.dp -> (maxWidth * 0.85f).coerceIn(780.dp, 880.dp)
                            maxWidth >= 768.dp -> (maxWidth * 0.92f).coerceIn(680.dp, 780.dp)
                            else -> maxWidth
                        }

                        LazyColumn(
                            state = listState,
                        modifier = Modifier.fillMaxSize(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        contentPadding = PaddingValues(top = 46.dp, bottom = 150.dp, start = 20.dp, end = 20.dp),
                        verticalArrangement = Arrangement.spacedBy(14.dp),
                    ) {
                        item {
                            Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                ContextHeader(
                                    cc = cc,
                                    discussion = discussion,
                                    onRenameDiscussion = { onIntent(ChatIntent.RenameDiscussion(it)) },
                                    onRegenerateTitle = { onIntent(ChatIntent.GenerateSummaryTitle) },
                                    typographySettings = typographySettings,
                                    evaluateConsensus = evaluateConsensus
                                )
                            }
                        }

                        if (discussion.mode == DiscussionMode.SOCRATIC_INTERVIEW) {
                            item(key = "socratic_epistemic_hud") {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    val stance = discussion.socraticConfig?.stance ?: SocraticStance.RUTHLESS_ELENCHUS
                                    val stage = state.socraticStage ?: discussion.socraticConfig?.stage ?: SocraticStage.HYPOTHESIS_EXTRACTION
                                    SocraticHud(
                                        stance = stance,
                                        stage = stage,
                                        interviewerName = discussion.socraticConfig?.interviewerName?.ifBlank { "Socratic Examiner" } ?: "Socratic Examiner",
                                        activeProbe = state.activeProbe,
                                        ledger = discussion.socraticLedger,
                                        isGeneratingDigest = state.isGeneratingDigest,
                                        onGenerateDigest = { onIntent(ChatIntent.GenerateSocraticDigest) }
                                    )
                                }
                            }
                        }

                        if (state.invalidFolders.isNotEmpty()) {
                            item(key = "invalid_folders_blocking_banner") {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    Surface(
                                        color = MaterialTheme.colorScheme.error.copy(alpha = 0.12f),
                                        shape = RoundedCornerShape(10.dp),
                                        border = BorderStroke(1.dp, MaterialTheme.colorScheme.error.copy(alpha = 0.6f)),
                                        modifier = Modifier.fillMaxWidth()
                                    ) {
                                        Column(modifier = Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                            Row(
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                                            ) {
                                                Icon(Icons.Outlined.FolderOff, contentDescription = null, tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(20.dp))
                                                Text(
                                                    "Workspace Folder Removed or Invalid — Processing Blocked",
                                                    style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.Bold, fontSize = 13.5.sp),
                                                    color = MaterialTheme.colorScheme.error
                                                )
                                            }
                                            Text(
                                                "One or more workspace folders attached to this discussion were removed or are inaccessible on disk. Discussion turns, resumes, and user comments are blocked until resolved.",
                                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                                color = MaterialTheme.colorScheme.error
                                            )
                                            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                                state.invalidFolders.forEach { (folder, reason) ->
                                                    Row(
                                                        modifier = Modifier
                                                            .fillMaxWidth()
                                                            .background(MaterialTheme.colorScheme.error.copy(alpha = 0.08f), RoundedCornerShape(6.dp))
                                                            .padding(horizontal = 10.dp, vertical = 6.dp),
                                                        horizontalArrangement = Arrangement.SpaceBetween,
                                                        verticalAlignment = Alignment.CenterVertically
                                                    ) {
                                                        Column(modifier = Modifier.weight(1f)) {
                                                            Text(
                                                                folder.path,
                                                                style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 12.sp),
                                                                color = MaterialTheme.colorScheme.error
                                                            )
                                                            Text(
                                                                reason,
                                                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                                                                color = MaterialTheme.colorScheme.error.copy(alpha = 0.8f)
                                                            )
                                                        }
                                                        Row(verticalAlignment = Alignment.CenterVertically) {
                                                            TextButton(
                                                                onClick = { onIntent(ChatIntent.RetryFolderValidation) },
                                                                contentPadding = PaddingValues(horizontal = 6.dp, vertical = 2.dp),
                                                                modifier = Modifier.height(26.dp)
                                                            ) {
                                                                Text("Retry", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Bold), color = MaterialTheme.colorScheme.error)
                                                            }
                                                            TextButton(
                                                                onClick = { onIntent(ChatIntent.RemoveInvalidFolder(folder.path)) },
                                                                contentPadding = PaddingValues(horizontal = 6.dp, vertical = 2.dp),
                                                                modifier = Modifier.height(26.dp)
                                                            ) {
                                                                Text("Remove", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Bold), color = MaterialTheme.colorScheme.error)
                                                            }
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }

                        // Pre-debate attached reference documents (rendered at top)
                        val preDebateArtifacts = discussion.artifacts.filter {
                            discussion.effectiveRoundFor(it) == null && it.id !in discussion.dismissedArtifactIds
                        }.sortedWith(compareBy<DiscussionArtifact> { if (it.timestampMs > 0L) it.timestampMs else 0L }.thenBy { it.id })

                        if (chatDisplaySettings.showArtifactGenerationStrips && preDebateArtifacts.isNotEmpty()) {
                            items(preDebateArtifacts.size, key = { "pre_debate_art_${preDebateArtifacts[it].id}" }) { idx ->
                                val art = preDebateArtifacts[idx]
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    ArtifactStrip(
                                        cc = cc,
                                        artifact = art,
                                        onView = toggleArtifactsPane,
                                        onDismiss = { onIntent(ChatIntent.DismissArtifactBanner(art.id)) }
                                    )
                                }
                            }
                        }

                        val primaryProvider = discussion.config.primary.provider
                        val compactionThreshold = 20

                        // All turns in strict chronological order
                        items(discussion.transcript.size) { i ->
                            val msg = discussion.transcript[i]
                            val prevMsg = if (i > 0) discussion.transcript[i - 1] else null
                            val nextMsg = if (i + 1 < discussion.transcript.size) discussion.transcript[i + 1] else null

                            val msgMatches = searchMatches.filter { it.transcriptIndex == i }
                            val isMatch = msgMatches.isNotEmpty()
                            val activeMatch = searchMatches.getOrNull(currentMatchIndex)
                            val isActiveMatch = activeMatch != null && activeMatch.transcriptIndex == i
                            val activeOccurrenceInMsg = if (isActiveMatch) activeMatch.occurrenceIndexInMessage else -1
                            val matchLabel = if (isActiveMatch) {
                                "MATCH ${currentMatchIndex + 1} of ${searchMatches.size}"
                            } else if (isMatch) {
                                if (msgMatches.size > 1) "${msgMatches.size} MATCHES" else "MATCH"
                            } else null

                            if (chatDisplaySettings.showCompactionPills && i > 0 && i % compactionThreshold == 0) {
                                CompactionInfoBar(
                                    cc = cc,
                                    startTurn = (i - compactionThreshold + 1).coerceAtLeast(1),
                                    endTurn = i,
                                    summaryText = discussion.summary?.takeIf { it.isNotBlank() }
                                )
                            }

                            val roundAgentTurns = discussion.transcript.take(i)
                                .filter { it.round == msg.round && !it.isUserComment && !it.isModeratorIntervention }
                            val seatIndex = if (discussion.config.agents.isNotEmpty()) {
                                roundAgentTurns.size % discussion.config.agents.size
                            } else {
                                0
                            }
                            val agentConfig = discussion.config.agents.find { it.id == msg.seatId }
                                ?: discussion.config.agents.getOrNull(seatIndex)
                                ?: discussion.config.agents.find { it.provider == msg.agentId }
                                ?: discussion.config.primary
                            val seatLabel = if (seatIndex >= 0) "Agent ${seatIndex + 1}" else if (msg.agentId == primaryProvider) "Primary" else ""
                            val memberDisplayName = discussion.labelFor(msg)

                            Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                if (prevMsg == null || msg.round != prevMsg.round) {
                                    RoundDivider(
                                        round = msg.round,
                                        totalRounds = discussion.config.maxRounds,
                                        cc = cc
                                    )
                                    val roundEvidence = state.retrievedEvidence.find { it.round == msg.round }
                                    if (roundEvidence != null && roundEvidence.items.isNotEmpty()) {
                                        Surface(
                                            shape = RoundedCornerShape(20.dp),
                                            color = cc.accent.copy(alpha = 0.08f),
                                            border = BorderStroke(0.5.dp, cc.accent.copy(alpha = 0.35f)),
                                            modifier = Modifier
                                                .align(Alignment.CenterHorizontally)
                                                .padding(vertical = 4.dp)
                                                .clickable { onIntent(ChatIntent.ToggleEvidenceDrawer(msg.round)) }
                                        ) {
                                            Row(
                                                modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                                            ) {
                                                Icon(
                                                    Icons.Outlined.TravelExplore,
                                                    contentDescription = null,
                                                    tint = cc.accent,
                                                    modifier = Modifier.size(13.dp)
                                                )
                                                Text(
                                                    text = "🔍 ${roundEvidence.items.size} Grounded Evidence Item${if (roundEvidence.items.size > 1) "s" else ""} for Round ${msg.round} · View Sources",
                                                    fontSize = 11.sp,
                                                    fontWeight = FontWeight.SemiBold,
                                                    color = cc.accent
                                                )
                                            }
                                        }
                                        Spacer(Modifier.height(4.dp))
                                    }
                                }

                                AeratedMessageItem(
                                    cc = cc,
                                    agentName = if (msg.isUserComment) "You (Observer)" else memberDisplayName,
                                    seatLabel = if (msg.isUserComment) "" else seatLabel,
                                    modelName = if (msg.isUserComment) "Human" else agentConfig.model,
                                    personaRole = if (msg.isUserComment) "User Comment" else agentConfig.role.takeIf { it.isNotBlank() },
                                    meta = if (msg.isError) "error" else "round ${msg.round}",
                                    content = msg.content,
                                    provider = msg.agentId,
                                    isPrimary = msg.agentId == primaryProvider,
                                    memberIndex = seatIndex,
                                    isError = msg.isError,
                                    tokensIn = msg.tokensIn,
                                    tokensOut = msg.tokensOut,
                                    typographySettings = typographySettings,
                                    isModeratorIntervention = msg.isModeratorIntervention,
                                    isUserComment = msg.isUserComment,
                                    isLoopRecovered = msg.isLoopRecovered,
                                    isStalledConcession = msg.isStalledConcession,
                                    agreed = msg.agreed,
                                    timestampMs = msg.timestampMs,
                                    searchQuery = if (isMatch) effectiveSearchQuery else null,
                                    activeOccurrenceIndex = activeOccurrenceInMsg,
                                    isSearchMatch = isMatch,
                                    isActiveSearchMatch = isActiveMatch,
                                    matchIndexLabel = matchLabel,
                                    isCliLoggedIn = run {
                                        val bin = extractCliBinary(msg.content, msg.provider)
                                        activeCliLogins[bin] == true || activeCliLogins[msg.provider.name] == true
                                    },
                                    onLoginCli = { binary ->
                                        val tool = SupportedCliTools.firstOrNull { it.binaryName == binary || it.name.contains(binary, ignoreCase = true) || it.id == binary }
                                        val cmd = tool?.loginCommand ?: binary
                                        launchCliLoginTerminal(cmd)
                                        toastScope.launch {
                                            snackbarHostState.showSnackbar("Terminal opened for '$cmd' login. Complete login in Terminal window.")
                                            repeat(30) {
                                                delay(2000)
                                                if (isPlatformCliSupported()) {
                                                    val updated = checkPlatformCliLogins()
                                                    activeCliLogins = updated
                                                    if (updated[binary] == true || updated[tool?.name.orEmpty()] == true || updated[tool?.provider?.name.orEmpty()] == true) {
                                                        snackbarHostState.showSnackbar("✓ $binary logged in successfully! Click Resume to continue.")
                                                        return@launch
                                                    }
                                                }
                                            }
                                        }
                                    },
                                    onRetry = {
                                        if (isPlatformCliSupported()) {
                                            activeCliLogins = checkPlatformCliLogins()
                                        }
                                        onIntent(ChatIntent.Resume)
                                    }
                                )

                                // Check if this message is the end of a milestone round with a saved deliverable or summary
                                val isEndOfRound = nextMsg == null || nextMsg.round != msg.round || nextMsg.isUserComment
                                val isFinalTurn = nextMsg == null
                                val milestoneRound = msg.round
                                val roundArtifacts = discussion.artifacts
                                    .filter { discussion.effectiveRoundFor(it) == milestoneRound }
                                    .sortedWith(compareBy<DiscussionArtifact> { if (it.timestampMs > 0L) it.timestampMs else 0L }.thenBy { it.id })
                                val maxCompletedAgentRound = discussion.transcript.filterNot { it.isUserComment || it.isError }.maxOfOrNull { it.round } ?: 1
                                val hasMilestoneDeliverable = roundArtifacts.any {
                                    it.type.equals("deliverable", ignoreCase = true) ||
                                    it.type.equals("summary", ignoreCase = true) ||
                                    it.id.startsWith("art_sum") ||
                                    it.id.startsWith("art_del")
                                } || (isFinalTurn && !msg.isUserComment && hasDeliverable) ||
                                  (isEndOfRound && !msg.isUserComment && milestoneRound == maxCompletedAgentRound && hasDeliverable)

                                if (isEndOfRound && !msg.isUserComment && (hasMilestoneDeliverable || (isFinalTurn && hasDeliverable))) {
                                    Spacer(Modifier.height(10.dp))
                                    CheckpointDivider(
                                        checkpointNumber = milestoneRound,
                                        cc = cc
                                    )
                                    Spacer(Modifier.height(6.dp))
                                    DeliverableBlock(
                                        cc = cc,
                                        discussion = discussion,
                                        typographySettings = typographySettings,
                                        milestoneRound = if (!isFinalTurn || roundArtifacts.any { it.round == milestoneRound || it.id.contains("_r$milestoneRound") }) milestoneRound else null,
                                        generatingFormat = if (isFinalTurn) state.generatingDeliverableFormat else null,
                                        onOpenSummary = toggleSummaryDialog,
                                        onOpenArtifacts = toggleArtifactsPane,
                                        onGenerateFormat = if (isFinalTurn) { fmt -> onIntent(ChatIntent.GenerateAlternativeDeliverable(fmt)) } else null,
                                        onExportDeliverable = { content, name ->
                                            val folder = discussion.config.output.outputFolder.ifBlank { "." }
                                            val ok = writeTextFile(folder, name, content)
                                            toastScope.launch {
                                                snackbarHostState.showSnackbar(
                                                    if (ok) "Saved $name to $folder" else "Failed to save $name",
                                                    duration = SnackbarDuration.Short
                                                )
                                            }
                                        }
                                    )
                                }

                                // Render milestone artifact notification strips right here in strict chronological order!
                                if (isEndOfRound && !msg.isUserComment && chatDisplaySettings.showArtifactGenerationStrips && roundArtifacts.isNotEmpty()) {
                                    roundArtifacts.filterNot { it.id in discussion.dismissedArtifactIds }.forEach { art ->
                                        Spacer(Modifier.height(4.dp))
                                        ArtifactStrip(
                                            cc = cc,
                                            artifact = art,
                                            onView = toggleArtifactsPane,
                                            onDismiss = { onIntent(ChatIntent.DismissArtifactBanner(art.id)) }
                                        )
                                    }
                                }
                            }
                        }

                        // Live typing / thinking bubble — or engine-disconnected error pill
                        if (state.engineConnectionLost && discussion.status == DiscussionStatus.RUNNING) {
                            item {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    StatusPill(cc, "⚠ Lost connection to engine — retrying…")
                                }
                            }
                        } else if (nextSpeakerAgent != null) {
                            item {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    AgentThinkingTicker(
                                        cc = cc,
                                        agent = nextSpeakerAgent,
                                        tokens = (discussion.transcript.sumOf { (it.tokensIn ?: 0) + (it.tokensOut ?: 0) })
                                    )
                                }
                            }
                        }

                        // Status messages
                        if (state.pauseRequested && discussion.status == DiscussionStatus.RUNNING) {
                            item {
                                StatusPill(cc, "Pausing — finishing current turn…")
                            }
                        } else if (discussion.status == DiscussionStatus.PAUSED) {
                            item {
                                StatusPill(cc, "Paused — click Resume to continue debate")
                            }
                        }

                        // AI Handoff Prompt
                        val primaryName = discussion.config.primary.label()
                        when (val h = handoff) {
                            is HandoffState.Loading -> item {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    AgentThinkingTicker(cc, discussion.config.primary, 0, action = "Synthesizing handoff prompt...")
                                }
                            }
                            is HandoffState.Failed -> item {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    AeratedMessageItem(
                                        cc = cc,
                                        agentName = primaryName,
                                        meta = "handoff prompt failed",
                                        content = h.message,
                                        provider = primaryProvider,
                                        isPrimary = true,
                                        isError = true,
                                        typographySettings = typographySettings
                                    )
                                }
                            }
                            else -> {
                                if (discussion.handoffPrompt != null) {
                                    item {
                                        Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                            AeratedMessageItem(
                                                cc = cc,
                                                agentName = primaryName,
                                                meta = "handoff prompt",
                                                content = discussion.handoffPrompt,
                                                provider = primaryProvider,
                                                isPrimary = true,
                                                typographySettings = typographySettings
                                            )
                                        }
                                    }
                                }
                            }
                        }

                        // Socratic Processing Indicator
                        if (state.isSocraticProcessing) {
                            item(key = "socratic_thinking_ticker") {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    AgentThinkingTicker(
                                        cc = cc,
                                        agent = discussion.config.primary,
                                        tokens = 0,
                                        action = "Interrogator is formulating probe..."
                                    )
                                }
                            }
                        }

                        // Socratic Digest Bubble (rendered if available)
                        val socraticDigest = state.socraticDigest ?: discussion.socraticDigest
                        if (socraticDigest != null) {
                            item(key = "socratic_digest_bubble") {
                                Column(Modifier.widthIn(max = maxContentWidth).fillMaxWidth()) {
                                    SocraticDigestBubble(
                                        digest = socraticDigest,
                                        isElevating = state.isElevatingToCouncil,
                                        onElevateToCouncil = {
                                            onIntent(ChatIntent.ElevateSocraticToCouncil())
                                        }
                                    )
                                }
                            }
                        }

                        // 36dp padding at the end of chat scroll
                        item {
                            Spacer(Modifier.height(36.dp))
                        }
                    }

                    // ── Floating Claude Bottom Input Bar (docked above bottom) ───
                Column(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                    .widthIn(max = maxContentWidth)
                    .fillMaxWidth()
                    .imePadding()
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(horizontal = if (isCompact) 12.dp else 20.dp, vertical = if (isCompact) 8.dp else 14.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                // Row 1: Dedicated Live Debate Controls Bar (Antigravity Style)
                val statusColor = when (discussion.status) {
                    DiscussionStatus.RUNNING -> Color(0xFF4CAF50)
                    DiscussionStatus.PAUSED -> Color(0xFFF59E0B)
                    DiscussionStatus.DONE, DiscussionStatus.COMPLETED -> cc.textMuted
                    DiscussionStatus.COMPLETED_WITH_WARNING -> Color(0xFFFFB300)
                    DiscussionStatus.ERROR, DiscussionStatus.FAILED -> Color(0xFFEF4444)
                    DiscussionStatus.DRAFT -> cc.textMuted
                }
                val isPulsing = discussion.status == DiscussionStatus.RUNNING || discussion.status == DiscussionStatus.PAUSED
                val infiniteTransition = rememberInfiniteTransition()
                val pulseAlpha by infiniteTransition.animateFloat(
                    initialValue = 0.2f,
                    targetValue = 0.65f,
                    animationSpec = infiniteRepeatable(
                        animation = tween(1200, easing = LinearEasing),
                        repeatMode = RepeatMode.Reverse
                    )
                )

                Surface(
                    shape = RoundedCornerShape(10.dp),
                    color = cc.panelAlt,
                    border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                    modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        // Left: Status Indicator, Glow Dot & Label
                        val isDebateError = discussion.status == DiscussionStatus.ERROR
                        val lastErrorTurn = remember(discussion.transcript) { discussion.transcript.findLast { it.isError } }
                        val errorSummary = lastErrorTurn?.content?.trim()
                            ?: latestAppError
                            ?: if (state.invalidFolders.isNotEmpty()) {
                                "Attached workspace context folder is missing or invalid on disk."
                            } else if (state.tokenProgressLabel.isNotEmpty() && state.tokenProgressLabel.contains("100%")) {
                                "Token budget limit reached. Raise budget in Settings > Limits to continue."
                            } else {
                                "Debate execution encountered an error. Click to view details."
                            }

                        Row(verticalAlignment = Alignment.CenterVertically) {
                            ThemedTooltipBox(
                                tooltip = if (isDebateError) "Error: ${errorSummary.take(130)}${if (errorSummary.length > 130) "…" else ""}\nClick to view error details" else ""
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    modifier = if (isDebateError) {
                                        Modifier
                                            .clip(RoundedCornerShape(6.dp))
                                            .clickable { isDebateErrorDialogOpen = true }
                                            .padding(horizontal = 4.dp, vertical = 2.dp)
                                    } else {
                                        Modifier
                                    }
                                ) {
                                    // Pulsating Glow dot with colored center & soft halo
                                    Box(
                                        modifier = Modifier.size(13.dp),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        if (isPulsing) {
                                            Box(
                                                modifier = Modifier
                                                    .size(12.dp)
                                                    .clip(CircleShape)
                                                    .background(statusColor.copy(alpha = pulseAlpha))
                                            )
                                        } else {
                                            Box(
                                                modifier = Modifier
                                                    .size(9.dp)
                                                    .clip(CircleShape)
                                                    .background(statusColor.copy(alpha = 0.2f))
                                            )
                                        }
                                        Box(
                                            modifier = Modifier
                                                .size(6.dp)
                                                .clip(CircleShape)
                                                .background(statusColor)
                                        )
                                    }
                                    Spacer(Modifier.width(8.dp))
                                    Text(
                                        text = when (discussion.status) {
                                            DiscussionStatus.RUNNING -> if (state.pauseRequested) "Pausing at turn end..." else "Debate in progress"
                                            DiscussionStatus.PAUSED -> "Debate paused"
                                            DiscussionStatus.DONE, DiscussionStatus.COMPLETED -> "Debate completed"
                                            DiscussionStatus.COMPLETED_WITH_WARNING -> "Deliberation concluded with warning"
                                            DiscussionStatus.ERROR, DiscussionStatus.FAILED -> "Debate error"
                                            DiscussionStatus.DRAFT -> "Draft debate"
                                        },
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, fontWeight = FontWeight.Normal),
                                        color = if (isDebateError) Color(0xFFEF4444) else cc.textMuted
                                    )
                                    if (isDebateError) {
                                        Spacer(Modifier.width(5.dp))
                                        Icon(
                                            Icons.Outlined.Info,
                                            contentDescription = "View Error Details",
                                            tint = Color(0xFFEF4444),
                                            modifier = Modifier.size(13.dp)
                                        )
                                    }
                                }
                            }

                            Spacer(Modifier.width(10.dp))
                            val mode = discussion.config.userInterventionPolicy
                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = cc.panel,
                                border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f))
                            ) {
                                Text(
                                    when (mode) {
                                        com.dialex.model.UserInterventionPolicy.AUTONOMOUS_AUTOPILOT -> "Autopilot"
                                        com.dialex.model.UserInterventionPolicy.OBSERVER_INTERACTIVE -> "Interactive"
                                        com.dialex.model.UserInterventionPolicy.HUMAN_GATEKEEPER -> "Gatekeeper"
                                    },
                                    style = MaterialTheme.typography.labelSmall.copy(
                                        fontSize = 11.sp,
                                        fontWeight = FontWeight.Normal
                                    ),
                                    color = cc.textMuted,
                                    modifier = Modifier.padding(horizontal = 7.dp, vertical = 2.dp)
                                )
                            }

                            if (state.tokenProgressLabel.isNotBlank()) {
                                Spacer(Modifier.width(10.dp))
                                ThemedTooltipBox("Token safety budget (${state.tokenProgressLabel})\nClick to adjust limit settings") {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(5.dp))
                                            .clickable {
                                                if (onOpenTokenSettings != null) onOpenTokenSettings()
                                                else onOpenSettings?.invoke()
                                            }
                                            .padding(horizontal = 4.dp, vertical = 2.dp)
                                    ) {
                                        Icon(
                                            Icons.Outlined.Speed,
                                            contentDescription = "Token safety limits",
                                            tint = if (state.tokenWarningLevel == TokenWarningLevel.Critical || state.tokenWarningLevel == TokenWarningLevel.Exceeded) Color(0xFFEF4444) else cc.textMuted,
                                            modifier = Modifier.size(13.dp)
                                        )
                                        Spacer(Modifier.width(4.dp))
                                        Text(
                                            state.tokenProgressLabel,
                                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Normal),
                                            color = if (state.tokenWarningLevel == TokenWarningLevel.Critical || state.tokenWarningLevel == TokenWarningLevel.Exceeded) Color(0xFFEF4444) else cc.textMuted
                                        )
                                    }
                                }
                            }

                            if (state.totalSpendUsd > 0.0) {
                                Spacer(Modifier.width(6.dp))
                                val costColor = when {
                                    state.totalSpendUsd >= 2.00 -> Color(0xFFEF4444)
                                    state.totalSpendUsd >= 1.00 -> Color(0xFFF59E0B)
                                    else -> Color(0xFF10B981)
                                }
                                val formattedCost = "$" + (if (state.totalSpendUsd < 0.01) "<0.01" else (((state.totalSpendUsd * 100).toInt()) / 100.0).toString())
                                ThemedTooltipBox("Estimated total discussion spend: $formattedCost\nClick to inspect breakdown") {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(5.dp))
                                            .clickable { onIntent(ChatIntent.ShowUsageModal) }
                                            .padding(horizontal = 4.dp, vertical = 2.dp)
                                    ) {
                                        Text(
                                            formattedCost,
                                            style = MaterialTheme.typography.bodySmall.copy(
                                                fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                                                fontSize = 11.5.sp,
                                                fontWeight = FontWeight.SemiBold
                                            ),
                                            color = costColor
                                        )
                                    }
                                }
                            }

                            if (state.cachedTokensPercent > 0) {
                                Spacer(Modifier.width(6.dp))
                                ThemedTooltipBox("Prompt Cache Efficiency: ${state.cachedTokensPercent}% of input tokens were served from cache, reducing cost & latency") {
                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = Color(0xFF10B981).copy(alpha = 0.12f),
                                        border = BorderStroke(0.5.dp, Color(0xFF10B981).copy(alpha = 0.4f))
                                    ) {
                                        Text(
                                            "⚡ ${state.cachedTokensPercent}% cached",
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, fontWeight = FontWeight.SemiBold),
                                            color = if (cc.isDark) Color(0xFF6EE7B7) else Color(0xFF047857),
                                            modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp)
                                        )
                                    }
                                }
                            }

                            if (state.tensionPairs.isNotEmpty()) {
                                Spacer(Modifier.width(8.dp))
                                val openTensions = state.tensionPairs.count { it.status == TensionStatus.OPEN || it.status == TensionStatus.EXPLORED }
                                val badgeBg = if (openTensions > 0) Color(0xFFFEF3C7) else Color(0xFFD1FAE5)
                                val badgeText = if (openTensions > 0) Color(0xFFD97706) else Color(0xFF059669)
                                val badgeIcon = if (openTensions > 0) Icons.Outlined.ElectricBolt else Icons.Outlined.CheckCircle
                                val tooltip = if (openTensions > 0) {
                                    "$openTensions active dialectic tension(s) detected\nClick to inspect Tension Matrix"
                                } else {
                                    "All dialectic tensions synthesized\nClick to inspect Tension Matrix"
                                }
                                ThemedTooltipBox(tooltip) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(5.dp))
                                            .background(badgeBg)
                                            .clickable { onIntent(ChatIntent.ToggleTensionDrawer) }
                                            .padding(horizontal = 6.dp, vertical = 2.dp)
                                    ) {
                                        Icon(
                                            badgeIcon,
                                            contentDescription = "Tension Matrix",
                                            tint = badgeText,
                                            modifier = Modifier.size(12.dp)
                                        )
                                        Spacer(Modifier.width(4.dp))
                                        Text(
                                            text = if (openTensions > 0) "$openTensions Tension${if (openTensions > 1) "s" else ""}" else "Tensions Resolved",
                                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Bold),
                                            color = badgeText
                                        )
                                    }
                                }
                            }

                            if (state.retrievedEvidence.isNotEmpty()) {
                                Spacer(Modifier.width(8.dp))
                                val totalEvidenceCount = state.retrievedEvidence.sumOf { it.items.size }
                                val evBadgeBg = Color(0xFFEDE7F6)
                                val evBadgeText = Color(0xFF673AB7)
                                ThemedTooltipBox("$totalEvidenceCount grounded evidence items injected across rounds\nClick to inspect Dynamic Evidence Drawer") {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(5.dp))
                                            .background(evBadgeBg)
                                            .clickable { onIntent(ChatIntent.ToggleEvidenceDrawer(null)) }
                                            .padding(horizontal = 6.dp, vertical = 2.dp)
                                    ) {
                                        Icon(
                                            Icons.Outlined.TravelExplore,
                                            contentDescription = "Dynamic Evidence",
                                            tint = evBadgeText,
                                            modifier = Modifier.size(12.dp)
                                        )
                                        Spacer(Modifier.width(4.dp))
                                        Text(
                                            text = "$totalEvidenceCount Evidence",
                                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Bold),
                                            color = evBadgeText
                                        )
                                    }
                                }
                            }
                        }

                        // Right: Live Debate Controls
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            when (discussion.status) {
                                DiscussionStatus.RUNNING -> {
                                    Surface(
                                        shape = RoundedCornerShape(7.dp),
                                        color = cc.panel,
                                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                                        modifier = Modifier
                                            .height(30.dp)
                                            .clip(RoundedCornerShape(7.dp))
                                            .clickable(enabled = !state.isActionInProgress) {
                                                onIntent(ChatIntent.Pause)
                                            }
                                    ) {
                                        Row(
                                            modifier = Modifier.padding(horizontal = 11.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            Icon(Icons.Filled.Pause, contentDescription = "Pause", tint = cc.textMuted, modifier = Modifier.size(12.dp))
                                            Text("Pause", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                                        }
                                    }

                                    Surface(
                                        shape = RoundedCornerShape(7.dp),
                                        color = cc.panel,
                                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                                        modifier = Modifier
                                            .height(30.dp)
                                            .clip(RoundedCornerShape(7.dp))
                                            .clickable(enabled = !state.isActionInProgress) {
                                                onIntent(ChatIntent.HardStop)
                                            }
                                    ) {
                                        Row(
                                            modifier = Modifier.padding(horizontal = 11.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            Icon(Icons.Filled.Stop, contentDescription = "Stop", tint = cc.textMuted, modifier = Modifier.size(12.dp))
                                            Text("Stop", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                                        }
                                    }
                                }
                                DiscussionStatus.PAUSED, DiscussionStatus.ERROR -> {
                                    val canResume = !state.isActionInProgress && state.invalidFolders.isEmpty()
                                    Surface(
                                        shape = RoundedCornerShape(7.dp),
                                        color = if (canResume) cc.accent else cc.panel,
                                        border = if (canResume) null else BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                                        modifier = Modifier
                                            .height(30.dp)
                                            .clip(RoundedCornerShape(7.dp))
                                            .clickable(enabled = canResume) {
                                                onIntent(ChatIntent.Resume)
                                            }
                                    ) {
                                        Row(
                                            modifier = Modifier.padding(horizontal = 13.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            if (state.isActionInProgress) {
                                                CircularProgressIndicator(color = Color.White, modifier = Modifier.size(11.dp), strokeWidth = 1.5.dp)
                                            } else {
                                                Icon(Icons.Filled.PlayArrow, contentDescription = "Resume", tint = if (canResume) Color.White else cc.textMuted, modifier = Modifier.size(12.dp))
                                            }
                                            Text("Resume", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold), color = if (canResume) Color.White else cc.textMuted)
                                        }
                                    }

                                    Surface(
                                        shape = RoundedCornerShape(7.dp),
                                        color = cc.panel,
                                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                                        modifier = Modifier
                                            .height(30.dp)
                                            .clip(RoundedCornerShape(7.dp))
                                            .clickable(enabled = !state.isActionInProgress) {
                                                onIntent(ChatIntent.HardStop)
                                            }
                                    ) {
                                        Row(
                                            modifier = Modifier.padding(horizontal = 11.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            Icon(Icons.Filled.Stop, contentDescription = "Stop", tint = cc.textMuted, modifier = Modifier.size(12.dp))
                                            Text("Stop", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                                        }
                                    }
                                }
                                DiscussionStatus.DONE, DiscussionStatus.COMPLETED, DiscussionStatus.COMPLETED_WITH_WARNING -> {
                                    Surface(
                                        shape = RoundedCornerShape(7.dp),
                                        color = cc.panel,
                                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                                        modifier = Modifier
                                            .height(30.dp)
                                            .clip(RoundedCornerShape(7.dp))
                                            .clickable(enabled = !state.isActionInProgress) {
                                                isContinueDebateDialogOpen = true
                                            }
                                    ) {
                                        Row(
                                            modifier = Modifier.padding(horizontal = 12.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            Icon(Icons.Outlined.PlayArrow, contentDescription = "Continue Debate", tint = cc.textMuted, modifier = Modifier.size(13.dp))
                                            Text("Continue Debate", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                                        }
                                    }
                                }
                                else -> {}
                            }

                        }
                    }
                }

                // In-Flight Holding Queue Bubble (Holds queued comment while discussion is running)
                AnimatedVisibility(
                    visible = state.queuedUserComment != null,
                    enter = fadeIn() + expandVertically(),
                    exit = fadeOut() + shrinkVertically()
                ) {
                    val queuedText = state.queuedUserComment ?: ""
                    Surface(
                        shape = RoundedCornerShape(12.dp),
                        color = cc.panelAlt,
                        border = BorderStroke(1.dp, cc.accent.copy(alpha = 0.4f)),
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(bottom = 8.dp)
                    ) {
                        Column(
                            modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Surface(
                                        shape = CircleShape,
                                        color = cc.accent.copy(alpha = 0.15f),
                                        modifier = Modifier.size(18.dp)
                                    ) {
                                        Box(contentAlignment = Alignment.Center) {
                                            Icon(
                                                Icons.Outlined.HourglassTop,
                                                contentDescription = null,
                                                tint = cc.accent,
                                                modifier = Modifier.size(11.dp)
                                            )
                                        }
                                    }
                                    Text(
                                        "Queued for next turn",
                                        style = MaterialTheme.typography.labelSmall.copy(
                                            fontWeight = FontWeight.SemiBold,
                                            fontSize = 11.5.sp
                                        ),
                                        color = cc.accent
                                    )
                                }

                                IconButton(
                                    onClick = { onIntent(ChatIntent.CancelHeldComment) },
                                    modifier = Modifier.size(48.dp)
                                ) {
                                    Icon(
                                        Icons.Default.Close,
                                        contentDescription = "Cancel queued comment",
                                        tint = cc.textMuted,
                                        modifier = Modifier.size(14.dp)
                                    )
                                }
                            }

                            Spacer(Modifier.height(4.dp))

                            Text(
                                queuedText,
                                style = MaterialTheme.typography.bodySmall.copy(
                                    fontSize = 12.5.sp,
                                    lineHeight = 17.sp
                                ),
                                color = cc.textPrimary,
                                maxLines = 3,
                                overflow = TextOverflow.Ellipsis
                            )

                            Spacer(Modifier.height(8.dp))

                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.End,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Surface(
                                    shape = RoundedCornerShape(6.dp),
                                    color = cc.accent,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(6.dp))
                                        .clickable {
                                            onIntent(ChatIntent.InterruptAndSendHeldComment)
                                        }
                                ) {
                                    Row(
                                        modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                                    ) {
                                        Icon(
                                            Icons.Filled.Bolt,
                                            contentDescription = null,
                                            tint = Color.White,
                                            modifier = Modifier.size(12.dp)
                                        )
                                        Text(
                                            "Interrupt & Send Now",
                                            style = MaterialTheme.typography.labelSmall.copy(
                                                fontWeight = FontWeight.SemiBold,
                                                fontSize = 11.5.sp
                                            ),
                                            color = Color.White
                                        )
                                    }
                                }
                            }
                        }
                    }
                }

                // Row 2: User Input Box with Round Arrow Send Button
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(14.dp))
                        .background(cc.panel)
                        .border(BorderStroke(1.dp, cc.border), RoundedCornerShape(14.dp))
                        .padding(horizontal = 14.dp, vertical = 10.dp)
                ) {
                    // Socratic Dialogue Assist Chips
                    if (discussion.mode == DiscussionMode.SOCRATIC_INTERVIEW) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .horizontalScroll(rememberScrollState())
                                .padding(bottom = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Surface(
                                shape = RoundedCornerShape(12.dp),
                                color = cc.panel,
                                border = BorderStroke(0.75.dp, cc.border),
                                modifier = Modifier.clickable {
                                    inputPrompt = (if (inputPrompt.isBlank()) "" else "$inputPrompt ") + "Consider the failure mode where "
                                }
                            ) {
                                Text(
                                    "⚡ Expose failure mode",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium),
                                    color = cc.textPrimary,
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                                )
                            }

                            Surface(
                                shape = RoundedCornerShape(12.dp),
                                color = cc.panel,
                                border = BorderStroke(0.75.dp, cc.border),
                                modifier = Modifier.clickable {
                                    inputPrompt = (if (inputPrompt.isBlank()) "" else "$inputPrompt ") + "The non-negotiable invariant is that "
                                }
                            ) {
                                Text(
                                    "🛡️ Defend invariant",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium),
                                    color = cc.textPrimary,
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                                )
                            }

                            Surface(
                                shape = RoundedCornerShape(12.dp),
                                color = cc.panel,
                                border = BorderStroke(0.75.dp, cc.border),
                                modifier = Modifier.clickable {
                                    inputPrompt = (if (inputPrompt.isBlank()) "" else "$inputPrompt ") + "I concede that under extreme load, "
                                }
                            ) {
                                Text(
                                    "🏳️ Concede constraint",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium),
                                    color = cc.textPrimary,
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                                )
                            }

                            Surface(
                                shape = RoundedCornerShape(12.dp),
                                color = Color(0xFFF59E0B).copy(alpha = 0.12f),
                                border = BorderStroke(0.75.dp, Color(0xFFF59E0B).copy(alpha = 0.4f)),
                                modifier = Modifier.clickable(enabled = !state.isGeneratingDigest) {
                                    onIntent(ChatIntent.GenerateSocraticDigest)
                                }
                            ) {
                                Text(
                                    if (state.isGeneratingDigest) "⏳ Synthesizing..." else "📜 Synthesize Digest",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold),
                                    color = Color(0xFFF59E0B),
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                                )
                            }
                        }
                    }

                    // Input Text Field
                    BasicTextField(
                        value = inputPrompt,
                        onValueChange = { inputPrompt = it },
                        textStyle = MaterialTheme.typography.bodyMedium.copy(color = cc.textPrimary, lineHeight = 20.sp),
                        cursorBrush = SolidColor(cc.accent),
                        minLines = 1,
                        maxLines = 4,
                        modifier = Modifier.fillMaxWidth(),
                        decorationBox = { innerTextField ->
                            if (inputPrompt.isEmpty()) {
                                Text(
                                    if (discussion.mode == DiscussionMode.SOCRATIC_INTERVIEW)
                                        "Articulate your thesis, defend an invariant, or answer probe..."
                                    else if (discussion.config.userInterventionPolicy == com.dialex.model.UserInterventionPolicy.AUTONOMOUS_AUTOPILOT)
                                        "⚡ Autopilot active — type here to inject a human comment or override..."
                                    else
                                        "Add a comment or inject a point for the agents...",
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = cc.textMuted.copy(alpha = 0.6f)
                                )
                            }
                            innerTextField()
                        }
                    )

                    Spacer(Modifier.height(8.dp))

                    // Input Controls Row: Mode selector, Attach, Voice, and Round Send Button
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        if (discussion.mode == DiscussionMode.SOCRATIC_INTERVIEW) {
                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = Color(0xFFF59E0B).copy(alpha = 0.12f),
                                border = BorderStroke(0.75.dp, Color(0xFFF59E0B).copy(alpha = 0.35f))
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        Icons.Outlined.TipsAndUpdates,
                                        contentDescription = null,
                                        tint = Color(0xFFF59E0B),
                                        modifier = Modifier.size(13.dp)
                                    )
                                    Text(
                                        "1-on-1 Socratic Turn",
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold),
                                        color = Color(0xFFF59E0B)
                                    )
                                }
                            }
                        } else {
                            // Left: Mode / Target Agent Pill
                            Box {
                                Row(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(6.dp))
                                        .clickable { agentModeDropdownOpen = true }
                                        .padding(horizontal = 8.dp, vertical = 6.dp),
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Text(
                                        selectedAgentMode,
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 12.sp, fontWeight = FontWeight.Medium),
                                        color = cc.textMuted
                                    )
                                    Spacer(Modifier.width(2.dp))
                                    Icon(
                                        Icons.Default.KeyboardArrowDown,
                                        contentDescription = null,
                                        tint = cc.textMuted,
                                        modifier = Modifier.size(14.dp)
                                    )
                                }

                                DropdownMenu(
                                    expanded = agentModeDropdownOpen,
                                    onDismissRequest = { agentModeDropdownOpen = false }
                                ) {
                                    DropdownMenuItem(
                                        text = { Text("All Agents (Open Comment)") },
                                        onClick = {
                                            selectedAgentMode = "All Agents"
                                            agentModeDropdownOpen = false
                                        }
                                    )
                                    discussion.config.agents.forEach { agent ->
                                        DropdownMenuItem(
                                            text = { Text(agent.label()) },
                                            onClick = {
                                                selectedAgentMode = agent.label()
                                                agentModeDropdownOpen = false
                                            }
                                        )
                                    }
                                }
                            }
                        }

                        // Right: Attach +, Voice 🎙, Round Send Arrow Button
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            IconButton(onClick = { /* Attach */ }, modifier = Modifier.size(48.dp)) {
                                Icon(Icons.Default.Add, contentDescription = "Attach file", tint = cc.textMuted, modifier = Modifier.size(20.dp))
                            }
                            com.dialex.ui.VoiceInputButton(
                                currentText = inputPrompt,
                                onTextChange = { inputPrompt = it },
                                size = 48.dp,
                                iconSize = 20.dp
                            )

                            Spacer(Modifier.width(2.dp))

                            // Round Send Button with Right Arrow (User Comments)
                            val canSendComment = inputPrompt.isNotBlank() && state.invalidFolders.isEmpty() && !state.isSocraticProcessing
                            Surface(
                                shape = CircleShape,
                                color = if (canSendComment) cc.accent else cc.panelAlt,
                                border = BorderStroke(0.75.dp, if (canSendComment) cc.accent else cc.border.copy(alpha = 0.5f)),
                                modifier = Modifier
                                    .size(if (isCompact) 36.dp else 28.dp)
                                    .clip(CircleShape)
                                    .clickable(enabled = canSendComment) {
                                        onIntent(ChatIntent.SendUserComment(inputPrompt))
                                        inputPrompt = ""
                                    }
                            ) {
                                Box(contentAlignment = Alignment.Center) {
                                    if (state.isSocraticProcessing) {
                                        CircularProgressIndicator(
                                            modifier = Modifier.size(if (isCompact) 18.dp else 14.dp),
                                            strokeWidth = 2.dp,
                                            color = Color.White
                                        )
                                    } else {
                                        Icon(
                                            Icons.AutoMirrored.Filled.ArrowForward,
                                            contentDescription = "Send User Comment",
                                            tint = if (canSendComment) Color.White else cc.textMuted.copy(alpha = 0.5f),
                                            modifier = Modifier.size(if (isCompact) 18.dp else 14.dp)
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // ── Floating Navigation Action Buttons (Deliverable Checkpoint & Scroll to Bottom) ───
            val deliverableTargetIndex = 1 + discussion.transcript.size
            val isDeliverableVisible by remember(hasDeliverable, deliverableTargetIndex) {
                derivedStateOf {
                    listState.layoutInfo.visibleItemsInfo.any { it.index == deliverableTargetIndex }
                }
            }
            val isPastDeliverable by remember(hasDeliverable, deliverableTargetIndex) {
                derivedStateOf {
                    listState.firstVisibleItemIndex >= deliverableTargetIndex
                }
            }

            Column(
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .padding(
                        end = if (isCompact) 16.dp else 28.dp,
                        bottom = if (isCompact) 115.dp else 125.dp
                    ),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                horizontalAlignment = Alignment.End
            ) {
                // Deliverable Jump FAB
                androidx.compose.animation.AnimatedVisibility(
                    visible = hasDeliverable && !isDeliverableVisible,
                    enter = fadeIn() + expandVertically(),
                    exit = fadeOut() + shrinkVertically()
                ) {
                    val tooltipText = if (isPastDeliverable) "Go to last deliverable (scroll up)" else "Go to next deliverable (scroll down)"
                    ThemedTooltipBox(tooltipText) {
                        Surface(
                            shape = RoundedCornerShape(20.dp),
                            color = cc.panel,
                            tonalElevation = 6.dp,
                            shadowElevation = 6.dp,
                            border = BorderStroke(1.dp, cc.accent.copy(alpha = 0.65f)),
                            modifier = Modifier
                                .clip(RoundedCornerShape(20.dp))
                                .clickable {
                                    toastScope.launch {
                                        listState.animateScrollToItem(deliverableTargetIndex.coerceAtLeast(0))
                                    }
                                }
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 11.dp, vertical = 6.5.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(5.dp)
                            ) {
                                Icon(
                                    imageVector = if (isPastDeliverable) Icons.Outlined.KeyboardDoubleArrowUp else Icons.Outlined.KeyboardDoubleArrowDown,
                                    contentDescription = tooltipText,
                                    tint = cc.accent,
                                    modifier = Modifier.size(16.dp)
                                )
                                Text(
                                    text = "Deliverable",
                                    style = MaterialTheme.typography.labelSmall.copy(
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 11.5.sp
                                    ),
                                    color = cc.accent
                                )
                            }
                        }
                    }
                }

                // Scroll navigation buttons (Top & Bottom FABs)
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    // Scroll to top FAB (hidden when at the top)
                    androidx.compose.animation.AnimatedVisibility(
                        visible = listState.canScrollBackward,
                        enter = fadeIn(),
                        exit = fadeOut()
                    ) {
                        ThemedTooltipBox("Scroll to top") {
                            FloatingActionButton(
                                onClick = {
                                    toastScope.launch {
                                        listState.animateScrollToItem(0)
                                    }
                                },
                                containerColor = cc.panelAlt,
                                contentColor = cc.textPrimary,
                                modifier = Modifier.size(36.dp),
                                shape = CircleShape
                            ) {
                                Icon(
                                    Icons.Filled.KeyboardArrowUp,
                                    contentDescription = "Scroll to top",
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                        }
                    }

                    // Scroll to bottom FAB (hidden when at the bottom)
                    androidx.compose.animation.AnimatedVisibility(
                        visible = listState.canScrollForward,
                        enter = fadeIn(),
                        exit = fadeOut()
                    ) {
                        ThemedTooltipBox("Scroll to bottom") {
                            FloatingActionButton(
                                onClick = {
                                    toastScope.launch {
                                        val count = listState.layoutInfo.totalItemsCount
                                        if (count > 0) {
                                            listState.animateScrollToItem(count - 1)
                                        }
                                    }
                                },
                                containerColor = cc.panelAlt,
                                contentColor = cc.textPrimary,
                                modifier = Modifier.size(36.dp),
                                shape = CircleShape
                            ) {
                                Icon(
                                    Icons.Filled.KeyboardArrowDown,
                                    contentDescription = "Scroll to bottom",
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                        }
                    }
                }
            }

            com.dialex.ui.ThemedSnackbarHost(snackbarHostState, Modifier.align(Alignment.BottomCenter).padding(bottom = 120.dp))
        } // End of discussion space Box(Modifier.weight(1f).fillMaxHeight())
    } // End of Row(Modifier.weight(1f).fillMaxWidth())
} // End of Column(Modifier.fillMaxSize())

            if (state.showUsageModal) {
                UsageModal(
                    usageBreakdown = state.usageBreakdown,
                    onDismiss = { onIntent(ChatIntent.DismissUsageModal) },
                    cc = cc
                )
            }

            if (state.isTensionDrawerOpen) {
                TensionMatrixDrawer(
                    tensions = state.tensionPairs,
                    filter = state.tensionFilter,
                    onFilterSelect = { onIntent(ChatIntent.SetTensionFilter(it)) },
                    onDismiss = { onIntent(ChatIntent.SetTensionDrawerOpen(false)) }
                )
            }

            if (state.isEvidenceDrawerOpen) {
                RoundEvidenceDrawer(
                    evidenceList = state.retrievedEvidence,
                    selectedRound = state.selectedEvidenceRound,
                    onRoundSelect = { onIntent(ChatIntent.ToggleEvidenceDrawer(it)) },
                    onDismiss = { onIntent(ChatIntent.SetEvidenceDrawerOpen(false)) }
                )
            }

            val credenceLedger = state.credenceLedger
            if (state.isCredenceDrawerOpen && credenceLedger != null) {
                CredenceDrawer(
                    ledger = credenceLedger,
                    selectedRound = state.selectedCredenceRound,
                    isRecalculating = state.isRecalculatingCredence,
                    onSelectRound = { onIntent(ChatIntent.SelectCredenceRound(it)) },
                    onRecalculate = { onIntent(ChatIntent.RecalculateCredence) },
                    onDismiss = { onIntent(ChatIntent.SetCredenceDrawerOpen(false)) }
                )
            }

            // ── Slide-Over Overlapping Drawer: Artifacts Repository ───
            val isArtifactsOpen = isArtifactsPaneOpen || state.artifactsModalOpen
            val closeArtifacts: () -> Unit = {
                if (state.artifactsModalOpen) onIntent(ChatIntent.ToggleArtifactsModal)
                if (isArtifactsPaneOpen) toggleArtifactsPane()
            }

            // Dimmed Scrim Backdrop
            AnimatedVisibility(
                visible = isArtifactsOpen,
                enter = fadeIn(),
                exit = fadeOut()
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .background(Color.Black.copy(alpha = if (cc.isDark) 0.45f else 0.25f))
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null,
                            onClick = closeArtifacts
                        )
                )
            }

            // Right-anchored Drawer
            AnimatedVisibility(
                visible = isArtifactsOpen,
                enter = slideInHorizontally(initialOffsetX = { it }) + fadeIn(),
                exit = slideOutHorizontally(targetOffsetX = { it }) + fadeOut(),
                modifier = Modifier.align(Alignment.CenterEnd).fillMaxHeight()
            ) {
                val drawerWidth = when {
                    maxWidth < 600.dp -> maxWidth * 0.92f
                    maxWidth < 1100.dp -> 480.dp
                    maxWidth < 1500.dp -> 540.dp
                    else -> 600.dp
                }

                Surface(
                    shape = RoundedCornerShape(topStart = 16.dp, bottomStart = 16.dp),
                    color = cc.panel,
                    shadowElevation = 16.dp,
                    border = BorderStroke(1.dp, cc.border.copy(alpha = 0.65f)),
                    modifier = Modifier
                        .width(drawerWidth)
                        .fillMaxHeight()
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null
                        ) { /* Consume clicks inside drawer */ }
                ) {
                    ArtifactsSlidingPane(
                        discussion = discussion,
                        onClose = closeArtifacts,
                        onExport = { content, name ->
                            val folder = discussion.config.output.outputFolder.ifBlank { "." }
                            val ok = writeTextFile(folder, name, content)
                            toastScope.launch {
                                snackbarHostState.showSnackbar(
                                    if (ok) "Saved $name to $folder" else "Failed to save $name",
                                    duration = SnackbarDuration.Short
                                )
                            }
                        },
                        typographySettings = typographySettings,
                        cc = cc,
                        modifier = Modifier.fillMaxSize(),
                        onDeleteArtifact = { onIntent(ChatIntent.DeleteArtifact(it)) }
                    )
                }
            }

            if (isSummaryDialogOpen) {
                DiscussionSummaryDialog(
                    discussion = discussion,
                    onDismiss = toggleSummaryDialog,
                    onExportMarkdown = { content, name ->
                        val folder = discussion.config.output.outputFolder.ifBlank { "." }
                        val ok = writeTextFile(folder, name, content)
                        toastScope.launch {
                            snackbarHostState.showSnackbar(
                                if (ok) "Saved $name to $folder" else "Failed to save $name",
                                duration = SnackbarDuration.Short
                            )
                        }
                    },
                    typographySettings = typographySettings,
                    cc = cc
                )
            }

            if (isDebateErrorDialogOpen && discussion.status == DiscussionStatus.ERROR) {
                val lastErrorTurn = remember(discussion.transcript) { discussion.transcript.findLast { it.isError } }
                val errorContent = lastErrorTurn?.content?.trim()
                    ?: latestAppError
                    ?: if (state.invalidFolders.isNotEmpty()) {
                        "One or more attached workspace folders could not be located on disk:\n\n" +
                            state.invalidFolders.joinToString("\n") { "• ${it.first.path}: ${it.second}" } +
                            "\n\nPlease resolve the folder paths in Setup, then click Resume."
                    } else if (state.tokenProgressLabel.isNotEmpty() && state.tokenProgressLabel.contains("100%")) {
                        "The discussion has reached its configured safety Token Budget (${state.tokenProgressLabel}).\n\n" +
                            "To continue this debate, open Settings > Limits, increase 'Max tokens per discussion' (or set to 0 to disable), and click Resume."
                    } else {
                        "Debate execution encountered an error.\n\n" +
                            "Possible causes:\n" +
                            "• Model API key missing or expired (check Settings > Providers)\n" +
                            "• Local CLI executable (claude, codex, gemini) not installed or requires auth\n" +
                            "• Backend engine communication issue or timeout\n\n" +
                            "Click 'Resume' to retry the last turn or review App Logs in Settings."
                    }
                DebateErrorDialog(
                    discussion = discussion,
                    errorMessage = errorContent,
                    agentProvider = lastErrorTurn?.agentId,
                    round = lastErrorTurn?.round,
                    onDismiss = { isDebateErrorDialogOpen = false },
                    onResume = {
                        isDebateErrorDialogOpen = false
                        onIntent(ChatIntent.Resume)
                    },
                    cc = cc
                )
            }

            if (isContinueDebateDialogOpen) {
                ContinueDebateDialog(
                    cc = cc,
                    currentMaxRound = discussion.transcript.maxOfOrNull { it.round } ?: 1,
                    configuredMaxRounds = discussion.config.maxRounds,
                    isUnlimited = discussion.config.roundMode == RoundMode.UNLIMITED,
                    onDismiss = { isContinueDebateDialogOpen = false },
                    onConfirm = { additionalRounds, unlimited ->
                        isContinueDebateDialogOpen = false
                        onIntent(ChatIntent.RestartDebate(additionalRounds, unlimited))
                    }
                )
            }

            if (activeAuthCliTool != null) {
                CliAuthDialog(
                    tool = activeAuthCliTool!!,
                    onDismiss = { activeAuthCliTool = null },
                    onSuccess = {
                        activeAuthCliTool = null
                        onIntent(ChatIntent.Resume)
                    }
                )
            }
        }
    }
}
