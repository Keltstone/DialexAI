package com.dialex.presentation.setup

import com.dialex.model.CouncilTemplate
import com.dialex.model.DebateConfig
import com.dialex.model.Discussion
import com.dialex.model.PersonaSelectionResult
import com.dialex.model.PredefinedPersona
import com.dialex.model.Project
import com.dialex.model.Provider
import com.dialex.model.PresetArchetype
import com.dialex.orchestrator.DeliberationEstimator
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.ImmutableMap
import kotlinx.collections.immutable.ImmutableSet
import kotlinx.collections.immutable.persistentListOf
import kotlinx.collections.immutable.persistentMapOf
import kotlinx.collections.immutable.persistentSetOf
// Setup Screen Contract — State / Intent / Effect
enum class SetupStep {
    /** Front page displaying one-click deliberation archetypes & quick start options. */
    FrontPage,
    /** Detailed discussion configuration form. */
    ConfigForm
}

/**
 * Progressive token warning level — computed once in the ViewModel from the current token
 * usage vs budget, written into [ChatState]. Composable only reads this field and renders
 * the appropriate color/label without doing any arithmetic itself.
 */
enum class TokenWarningLevel {
    /** Under 50% — neutral, no warning shown. */
    None,
    /** 50–64% — subtle gray pill. */
    Caution,
    /** 65–79% — amber pill. */
    Warning,
    /** 80–94% — orange pill. */
    High,
    /** 95–99% — critical red pill. */
    Critical,
    /** 100% / over budget — hard-stop red badge, discussion halted. */
    Exceeded,
}

data class SetupState(
    /** Current step in the setup flow (FrontPage or ConfigForm). */
    val step: SetupStep = SetupStep.FrontPage,
    /** Available one-click templates (built-in archetypes + user custom templates). */
    val templates: ImmutableList<CouncilTemplate> = persistentListOf(),
    /** Currently applied council template archetype, if one was selected from the Front Page. */
    val selectedTemplate: CouncilTemplate? = null,
    /** When non-null, the full-screen Persona Picker is open for this agent index (0=primary, 1=secondary, ...). */
    val activeAgentForPersonaPicker: Int? = null,
    /** All projects available for selection in the project picker. */
    val projects: ImmutableList<Project> = persistentListOf(),
    /** Currently selected project. */
    val selectedProject: Project? = null,
    /** The discussion being configured (may be an unsaved draft or an existing one). */
    val discussion: Discussion? = null,
    /** All discussions in the current project (for the "Copy settings from" flow). */
    val otherDiscussions: ImmutableList<Discussion> = persistentListOf(),
    /** Available personas for the Persona Picker in each agent seat. */
    val availablePersonas: ImmutableList<PredefinedPersona> = persistentListOf(),
    /** Available models per provider (queried from engine or default catalog). */
    val availableModels: ImmutableMap<String, ImmutableList<String>> = persistentMapOf(),
    /** Providers with configured API keys. */
    val configuredApiProviders: ImmutableSet<Provider> = persistentSetOf(),
    /** Providers with CLI available on the system running the engine (local or remote). */
    val availableCliProviders: ImmutableSet<Provider> = persistentSetOf(),
    /** Whether CLI mode is available on this platform (false on Android). */
    val supportsCli: Boolean = false,
    /** Validation errors blocking start; empty = can start. */
    val validationErrors: ImmutableList<String> = persistentListOf(),
    /** Folders that were removed, missing, or invalid on disk paired with the failure reason. */
    val invalidFolders: ImmutableList<Pair<com.dialex.model.FolderScope, String>> = persistentListOf(),
    /** Whether the project-level inherited SharedContext/SharedInstructions banner is visible. */
    val showProjectContextBanner: Boolean = false,
    /** Active intent-driven archetype preset (Task-08). */
    val activeArchetype: PresetArchetype = PresetArchetype.EXECUTIVE_DECISION,
    /** Real-time cost, token, and latency impact preview (Task-08). */
    val runEstimate: DeliberationEstimator.RunEstimate? = null,
    /** Whether Level 3: Power-User Drawer (Advanced Engine & Tuning) is expanded. */
    val advancedExpanded: Boolean = false,
    val loadAsync: AsyncState<Unit> = AsyncState.Idle,
    val saveAsync: AsyncState<Unit> = AsyncState.Idle,
    /** Whether problem decomposition analysis is currently executing. */
    val isDecomposing: Boolean = false,
    /** The active decomposition result generated for the current topic. */
    val activeDecomposition: com.dialex.domain.model.ProblemDecomposition? = null,
    /** Whether the decomposition inspection and selection modal is displayed. */
    val showDecompositionSheet: Boolean = false,
    /** Set of selected axis IDs to be included in the debate agenda. */
    val selectedAxisIds: ImmutableSet<String> = persistentSetOf(),
    /** Operating mode: COUNCIL (multi-agent) vs SOCRATIC_INTERVIEW (1-on-1). */
    val mode: com.dialex.domain.model.DiscussionMode = com.dialex.domain.model.DiscussionMode.COUNCIL,
    /** Active Socratic stance when in SOCRATIC_INTERVIEW mode. */
    val socraticStance: com.dialex.domain.model.SocraticStance = com.dialex.domain.model.SocraticStance.RUTHLESS_ELENCHUS,
    /** Preselected interviewer persona for Socratic mode. */
    val socraticInterviewer: PredefinedPersona? = null,
)

sealed interface SetupIntent {
    data class SetDiscussionMode(val mode: com.dialex.domain.model.DiscussionMode) : SetupIntent
    data class SelectSocraticStance(val stance: com.dialex.domain.model.SocraticStance) : SetupIntent
    data class SelectSocraticInterviewer(val persona: PredefinedPersona) : SetupIntent
    data object AutoSuggestSocraticSetup : SetupIntent
    data class SelectProject(val projectId: String) : SetupIntent
    data class CreateProject(val name: String) : SetupIntent
    data class UpdateProjectContext(val sharedContext: String, val sharedInstructions: String) : SetupIntent
    data class DiscussionChanged(val discussion: Discussion) : SetupIntent
    data class ConfigChanged(val config: DebateConfig) : SetupIntent
    data class SelectArchetype(val archetype: PresetArchetype) : SetupIntent
    data class SelectOneKeyMode(val mode: com.dialex.model.OneKeyMode) : SetupIntent
    data object ToggleAdvancedDrawer : SetupIntent
    data class SelectTemplate(val template: CouncilTemplate) : SetupIntent
    data object SelectBlankConfig : SetupIntent
    data object NavigateToFrontPage : SetupIntent
    data class SaveCurrentAsTemplate(val title: String, val subtitle: String, val badgeLabel: String) : SetupIntent
    data class DeleteCustomTemplate(val templateId: String) : SetupIntent
    data class OpenPersonaPicker(val agentIndex: Int) : SetupIntent
    data object DismissPersonaPicker : SetupIntent
    data class ApplyPersonaToAgent(val agentIndex: Int, val result: PersonaSelectionResult) : SetupIntent
    data class AttachFile(val fileName: String, val content: String, val scope: String = "topic") : SetupIntent
    data class RemoveFile(val fileId: String) : SetupIntent
    data class AddFolder(val path: String, val isReadOnly: Boolean = false) : SetupIntent
    data class RemoveFolder(val path: String) : SetupIntent
    data class ToggleFolderTrust(val path: String) : SetupIntent
    data class ToggleDiscussionWebSearch(val enabled: Boolean) : SetupIntent
    data class ToggleAgentWebSearch(val agentIndex: Int, val enabled: Boolean) : SetupIntent
    data object RetryFolderValidation : SetupIntent
    data object RefreshAvailableModels : SetupIntent
    data class CopySettingsFrom(val sourceDiscussionId: String) : SetupIntent
    data object StartDiscussion : SetupIntent
    data object NavigateBack : SetupIntent
    data object OpenSettings : SetupIntent
    data object OpenPersonaBuilder : SetupIntent
    data object DismissError : SetupIntent
    data object RequestProblemDecomposition : SetupIntent
    data object DismissDecompositionSheet : SetupIntent
    data class ToggleAxisSelection(val axisId: String) : SetupIntent
    data object SelectAllPerspectiveA : SetupIntent
    data object SelectAllPerspectiveB : SetupIntent
    data object ApplyDecompositionToAgenda : SetupIntent
}

sealed interface SetupEffect {
    data class NavigateToChat(val discussionId: String) : SetupEffect
    data object NavigateBack : SetupEffect
    data object NavigateToSettings : SetupEffect
    data object NavigateToPersonaBuilder : SetupEffect
    data class ShowSnackbar(val message: String) : SetupEffect
    data object DecompositionApplied : SetupEffect
}
