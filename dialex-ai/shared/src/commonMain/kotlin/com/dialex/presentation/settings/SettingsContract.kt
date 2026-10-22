package com.dialex.presentation.settings

import com.dialex.model.ApiKeys
import com.dialex.model.CliCommands
import com.dialex.model.CompactionSettings
import com.dialex.model.PredefinedPersona
import com.dialex.model.Provider
import com.dialex.theme.ThemeMode
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.ImmutableMap
import kotlinx.collections.immutable.persistentListOf
import kotlinx.collections.immutable.persistentMapOf
// Settings Screen Contract — State / Intent / Effect
/** Live status of one provider's CLI tool and API key — shown in the AI Agent Hub grid. */
data class ProviderStatus(
    val provider: Provider,
    val cliFound: Boolean,
    val cliVersion: String?,
    val cliLoggedIn: Boolean,
    val apiKeyConfigured: Boolean,
)

data class SettingsState(
    val selectedTab: SettingsTab = SettingsTab.Appearance,
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    val connectionLabel: String = "",
    val supportsLocalEngine: Boolean = false,
    val activeProfile: com.dialex.domain.model.ConnectionProfile? = null,
    val availableProfiles: ImmutableList<com.dialex.domain.model.ConnectionProfile> = persistentListOf(),
    val showLockSetupDialog: Boolean = false,
    val replaceKeyProvider: Provider? = null,
    val apiKeys: ApiKeys = ApiKeys(),
    val cliCommands: CliCommands = CliCommands(),
    val compactionSettings: CompactionSettings = CompactionSettings(),
    val tokenBudget: Int = 1_000_000,
    val masterInstructions: String = "",
    val debatePolicy: com.dialex.model.DebatePolicy = com.dialex.model.DebatePolicy(),
    val agentDefaults: com.dialex.model.ProviderAgentDefaults = com.dialex.model.ProviderAgentDefaults(),
    val ollamaHealth: com.dialex.service.OllamaHealthStatus = com.dialex.service.OllamaHealthStatus.Checking,
    val installedOllamaModels: ImmutableList<com.dialex.service.OllamaModelInfo> = persistentListOf(),
    val isPullingOllamaModel: Boolean = false,
    val pullModelProgress: Float = 0f,
    val pullModelStatus: String = "",
    /** Live CLI/API status grid for the AI Agent Hub tab. */
    val providerStatuses: ImmutableList<ProviderStatus> = persistentListOf(),
    /** Personas available in the Personas tab. */
    val personas: ImmutableList<PredefinedPersona> = persistentListOf(),
    /** Available models per provider from the engine's /models endpoint. */
    val availableModels: ImmutableMap<String, ImmutableList<String>> = persistentMapOf(),
    val saveAsync: AsyncState<Unit> = AsyncState.Idle,
    val cliStatusAsync: AsyncState<Unit> = AsyncState.Idle,
    /** Confirmation dialog state for "Switch Connection". */
    val showSwitchConnectionConfirmation: Boolean = false,
    val exportJsonDialogContent: String? = null,
    val showImportDialog: Boolean = false,
    val showGalleryDialog: Boolean = false,
    val error: String? = null,
    val legalConsent: com.dialex.domain.model.LegalConsent = com.dialex.domain.model.LegalConsent.NotAccepted,
)

enum class SettingsTab {
    Appearance, Security, AiAgents, MasterInstructions, DebatePolicy, Compaction, Personas, Limits, Connection, Logs, About;

    val label: String
        get() = when (this) {
            Appearance -> "General"
            Security -> "Security & Lock"
            AiAgents -> "AI Agents"
            MasterInstructions -> "Master Instructions"
            DebatePolicy -> "Autopilot & Moderation"
            Compaction -> "Compaction"
            Personas -> "Personas"
            Limits -> "Limits"
            Connection -> "Connection"
            Logs -> "API & Server Logs"
            About -> "About & Legal"
        }
}

sealed interface SettingsIntent {
    data class SelectTab(val tab: SettingsTab) : SettingsIntent
    data class ThemeModeChanged(val mode: ThemeMode) : SettingsIntent
    data class ApiKeysChanged(val keys: ApiKeys) : SettingsIntent
    data class CliCommandsChanged(val commands: CliCommands) : SettingsIntent
    data class CompactionSettingsChanged(val settings: CompactionSettings) : SettingsIntent
    data class TokenBudgetChanged(val budget: Int) : SettingsIntent
    data class MasterInstructionsChanged(val instructions: String) : SettingsIntent
    data class DebatePolicyChanged(val policy: com.dialex.model.DebatePolicy) : SettingsIntent
    data class AgentDefaultsChanged(val defaults: com.dialex.model.ProviderAgentDefaults) : SettingsIntent
    data object Save : SettingsIntent
    data object RecheckCliStatus : SettingsIntent
    data object SwitchConnectionRequested : SettingsIntent
    data object SwitchConnectionConfirmed : SettingsIntent
    data object SwitchConnectionCancelled : SettingsIntent
    data class DeletePersona(val id: String) : SettingsIntent
    data class ImportPersonas(val json: String) : SettingsIntent
    data object ExportPersonas : SettingsIntent
    data class ExportSinglePersona(val persona: PredefinedPersona) : SettingsIntent
    data object DismissExportDialog : SettingsIntent
    data object OpenImportDialog : SettingsIntent
    data object DismissImportDialog : SettingsIntent
    data object OpenGalleryDialog : SettingsIntent
    data object DismissGalleryDialog : SettingsIntent
    data class InstallGalleryPersona(val persona: PredefinedPersona) : SettingsIntent
    data object OpenPersonaBuilder : SettingsIntent
    data class EditPersona(val id: String) : SettingsIntent
    data object DismissError : SettingsIntent

    // Security & Write-Only API Keys
    data class ReplaceKeyRequested(val provider: Provider) : SettingsIntent
    data class SetApiKey(val provider: Provider, val key: String) : SettingsIntent
    data class RemoveApiKey(val provider: Provider) : SettingsIntent
    data class UpdateProfileLock(val config: com.dialex.domain.model.ProfileLockConfig) : SettingsIntent
    data object ShowLockSetupDialog : SettingsIntent
    data object DismissLockSetupDialog : SettingsIntent

    // Ollama & Local LLMs
    data object RefreshOllama : SettingsIntent
    data class PullOllamaModel(val modelName: String) : SettingsIntent
    data class UpdateOllamaEndpoint(val endpoint: String) : SettingsIntent
    data class SwitchProfile(val profileId: String) : SettingsIntent
}

sealed interface SettingsEffect {
    data object SwitchConnection : SettingsEffect
    data class NavigateToPersonaBuilder(val personaId: String? = null) : SettingsEffect
    data class ShowSnackbar(val message: String) : SettingsEffect
    data class ExportPersonasJson(val json: String) : SettingsEffect
}
