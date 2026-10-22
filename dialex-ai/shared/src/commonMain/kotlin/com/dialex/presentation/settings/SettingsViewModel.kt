package com.dialex.presentation.settings

import androidx.lifecycle.viewModelScope
import com.dialex.domain.model.ConnectionProfile
import com.dialex.domain.model.ProfileLockConfig
import com.dialex.domain.repository.ApiKeyRepository
import com.dialex.domain.repository.PersonaRepository
import com.dialex.domain.repository.ProfileRepository
import com.dialex.domain.repository.SettingsRepository
import com.dialex.model.ApiKeys
import com.dialex.model.Provider
import com.dialex.model.SystemPersonas
import com.dialex.presentation.base.MviViewModel
import com.dialex.theme.ThemeMode
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.toImmutableList
import kotlinx.collections.immutable.toImmutableMap
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

class SettingsViewModel(
    private val settingsRepository: SettingsRepository,
    private val personaRepository: PersonaRepository,
    themeMode: ThemeMode,
    connectionLabel: String,
    supportsLocalEngine: Boolean,
    private val recheckCli: suspend () -> List<ProviderStatus>,
    initialTab: SettingsTab = SettingsTab.Appearance,
    private val profileRepository: ProfileRepository? = null,
    private val apiKeyRepository: ApiKeyRepository? = null,
    private val legalConsentRepository: com.dialex.domain.repository.LegalConsentRepository? = null
) : MviViewModel<SettingsState, SettingsIntent, SettingsEffect>(
    SettingsState(
        selectedTab = initialTab,
        themeMode = themeMode,
        connectionLabel = connectionLabel,
        supportsLocalEngine = supportsLocalEngine
    )
) {
    private val jsonSerializer = Json { prettyPrint = true }
    private val ollamaService = com.dialex.service.OllamaService()

    init {
        viewModelScope.launch {
            try {
                val settings = settingsRepository.getSettings()
                val personas = personaRepository.getPersonas()
                val models = settingsRepository.getAvailableModels()
                val activeProf = profileRepository?.getActiveProfile()
                val consent = legalConsentRepository?.getLegalConsent() ?: com.dialex.domain.model.LegalConsent.NotAccepted

                setState {
                    copy(
                        activeProfile = activeProf,
                        apiKeys = settings.apiKeys,
                        cliCommands = settings.cliCommands,
                        compactionSettings = settings.compactionSettings,
                        tokenBudget = settings.tokenBudget,
                        masterInstructions = settings.masterInstructions,
                        debatePolicy = settings.debatePolicy,
                        agentDefaults = settings.agentDefaults,
                        personas = (SystemPersonas + personas).distinctBy { it.id }.toImmutableList(),
                        availableModels = models.mapValues { entry -> entry.value.toImmutableList() }.toImmutableMap(),
                        legalConsent = consent
                    )
                }

                checkCliStatus()
                checkOllamaHealth(settings.apiKeys.ollama)
            } catch (e: Exception) {
                setState { copy(error = e.message ?: "Failed to load settings") }
            }
        }

        profileRepository?.getProfiles()?.onEach { profiles ->
            setState { copy(availableProfiles = profiles.toImmutableList()) }
        }?.launchIn(viewModelScope)
    }

    private suspend fun checkCliStatus() {
        setState { copy(cliStatusAsync = AsyncState.Loading) }
        try {
            val statuses = recheckCli()
            // Enrich with secure key database if available
            val activeId = state.value.activeProfile?.id
            val finalStatuses = if (apiKeyRepository != null && activeId != null) {
                val keyStatuses = apiKeyRepository.getApiKeyStatuses(activeId)
                statuses.map { status ->
                    val isConfigured = keyStatuses.isConfigured(status.provider) || status.apiKeyConfigured
                    status.copy(apiKeyConfigured = isConfigured)
                }
            } else {
                statuses
            }

            setState { copy(providerStatuses = finalStatuses.toImmutableList(), cliStatusAsync = AsyncState.Success(Unit)) }
        } catch (e: Exception) {
            setState { copy(cliStatusAsync = AsyncState.Error(e)) }
        }
    }

    override fun onIntent(intent: SettingsIntent) {
        when (intent) {
            is SettingsIntent.SelectTab -> setState { copy(selectedTab = intent.tab) }
            is SettingsIntent.ThemeModeChanged -> setState { copy(themeMode = intent.mode) }
            is SettingsIntent.ApiKeysChanged -> setState { copy(apiKeys = intent.keys) }
            is SettingsIntent.CliCommandsChanged -> setState { copy(cliCommands = intent.commands) }
            is SettingsIntent.CompactionSettingsChanged -> setState { copy(compactionSettings = intent.settings) }
            is SettingsIntent.TokenBudgetChanged -> setState { copy(tokenBudget = intent.budget) }
            is SettingsIntent.MasterInstructionsChanged -> setState { copy(masterInstructions = intent.instructions) }
            is SettingsIntent.DebatePolicyChanged -> setState { copy(debatePolicy = intent.policy) }
            is SettingsIntent.AgentDefaultsChanged -> setState { copy(agentDefaults = intent.defaults) }
            is SettingsIntent.Save -> {
                viewModelScope.launch {
                    setState { copy(saveAsync = AsyncState.Loading) }
                    try {
                        settingsRepository.updateApiKeys(state.value.apiKeys)
                        settingsRepository.updateCliCommands(state.value.cliCommands)
                        settingsRepository.updateCompactionSettings(state.value.compactionSettings)
                        settingsRepository.updateTokenBudget(state.value.tokenBudget)
                        settingsRepository.updateMasterInstructions(state.value.masterInstructions)
                        settingsRepository.updateDebatePolicy(state.value.debatePolicy)
                        settingsRepository.updateAgentDefaults(state.value.agentDefaults)
                        setState { copy(saveAsync = AsyncState.Success(Unit)) }
                        sendEffect(SettingsEffect.ShowSnackbar("Settings saved successfully"))
                    } catch (e: Exception) {
                        setState { copy(saveAsync = AsyncState.Error(e), error = "Failed to save settings") }
                    }
                }
            }
            is SettingsIntent.RecheckCliStatus -> {
                viewModelScope.launch {
                    checkCliStatus()
                }
            }
            is SettingsIntent.SwitchConnectionRequested -> setState { copy(showSwitchConnectionConfirmation = true) }
            is SettingsIntent.SwitchConnectionConfirmed -> {
                setState { copy(showSwitchConnectionConfirmation = false) }
                sendEffect(SettingsEffect.SwitchConnection)
            }
            is SettingsIntent.SwitchConnectionCancelled -> setState { copy(showSwitchConnectionConfirmation = false) }
            is SettingsIntent.DeletePersona -> {
                viewModelScope.launch {
                    try {
                        personaRepository.deletePersona(intent.id)
                        val personas = personaRepository.getPersonas()
                        setState { copy(personas = (SystemPersonas + personas).distinctBy { it.id }.toImmutableList()) }
                        sendEffect(SettingsEffect.ShowSnackbar("Persona deleted"))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to delete persona") }
                    }
                }
            }
            is SettingsIntent.OpenImportDialog -> setState { copy(showImportDialog = true) }
            is SettingsIntent.OpenGalleryDialog -> setState { copy(showGalleryDialog = true) }
            is SettingsIntent.DismissGalleryDialog -> setState { copy(showGalleryDialog = false) }
            is SettingsIntent.InstallGalleryPersona -> {
                viewModelScope.launch {
                    try {
                        personaRepository.createPersona(intent.persona)
                        val personas = personaRepository.getPersonas()
                        setState {
                            copy(
                                personas = (SystemPersonas + personas).distinctBy { it.id }.toImmutableList()
                            )
                        }
                        sendEffect(SettingsEffect.ShowSnackbar("Installed ${intent.persona.name}"))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to install persona: ${e.message}") }
                    }
                }
            }
            is SettingsIntent.DismissImportDialog -> setState { copy(showImportDialog = false) }
            is SettingsIntent.ImportPersonas -> {
                viewModelScope.launch {
                    try {
                        val count = personaRepository.importPersonas(intent.json)
                        val personas = personaRepository.getPersonas()
                        setState { 
                            copy(
                                personas = (SystemPersonas + personas).distinctBy { it.id }.toImmutableList(),
                                showImportDialog = false
                            ) 
                        }
                        sendEffect(SettingsEffect.ShowSnackbar("Imported $count persona(s)"))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to import personas: ${e.message}") }
                    }
                }
            }
            is SettingsIntent.ExportPersonas -> {
                viewModelScope.launch {
                    try {
                        val json = personaRepository.exportPersonas()
                        val prettyJson = if (json.isNotBlank() && json.trim() != "[]") {
                            json
                        } else {
                            jsonSerializer.encodeToString(state.value.personas.toList())
                        }
                        setState { copy(exportJsonDialogContent = prettyJson) }
                        sendEffect(SettingsEffect.ExportPersonasJson(prettyJson))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to export personas") }
                    }
                }
            }
            is SettingsIntent.ExportSinglePersona -> {
                try {
                    val singleJson = jsonSerializer.encodeToString(listOf(intent.persona))
                    setState { copy(exportJsonDialogContent = singleJson) }
                    sendEffect(SettingsEffect.ExportPersonasJson(singleJson))
                } catch (e: Exception) {
                    setState { copy(error = "Failed to export persona") }
                }
            }
            is SettingsIntent.DismissExportDialog -> setState { copy(exportJsonDialogContent = null) }
            is SettingsIntent.OpenPersonaBuilder -> sendEffect(SettingsEffect.NavigateToPersonaBuilder(null))
            is SettingsIntent.EditPersona -> sendEffect(SettingsEffect.NavigateToPersonaBuilder(intent.id))
            is SettingsIntent.DismissError -> setState { copy(error = null) }

            // ── Security & Write-Only API Key Handlers ────────────────────────
            is SettingsIntent.ReplaceKeyRequested -> {
                setState { copy(replaceKeyProvider = intent.provider) }
            }
            is SettingsIntent.SetApiKey -> {
                val activeId = state.value.activeProfile?.id ?: ConnectionProfile.LOCAL_PROFILE_ID
                viewModelScope.launch {
                    try {
                        apiKeyRepository?.setApiKey(activeId, intent.provider, intent.key)
                        // Also update settings repository so the running engine has it
                        val updatedKeys = updateApiKeyMap(state.value.apiKeys, intent.provider, intent.key)
                        settingsRepository.updateApiKeys(updatedKeys)
                        setState { copy(apiKeys = updatedKeys, replaceKeyProvider = null) }
                        checkCliStatus()
                        sendEffect(SettingsEffect.ShowSnackbar("API key saved encrypted."))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to save API key: ${e.message}") }
                    }
                }
            }
            is SettingsIntent.RemoveApiKey -> {
                val activeId = state.value.activeProfile?.id ?: ConnectionProfile.LOCAL_PROFILE_ID
                viewModelScope.launch {
                    try {
                        apiKeyRepository?.removeApiKey(activeId, intent.provider)
                        val updatedKeys = updateApiKeyMap(state.value.apiKeys, intent.provider, "")
                        settingsRepository.updateApiKeys(updatedKeys)
                        setState { copy(apiKeys = updatedKeys) }
                        checkCliStatus()
                        sendEffect(SettingsEffect.ShowSnackbar("API key removed."))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to remove API key") }
                    }
                }
            }
            is SettingsIntent.ShowLockSetupDialog -> setState { copy(showLockSetupDialog = true) }
            is SettingsIntent.DismissLockSetupDialog -> setState { copy(showLockSetupDialog = false) }
            is SettingsIntent.UpdateProfileLock -> {
                val activeId = state.value.activeProfile?.id ?: ConnectionProfile.LOCAL_PROFILE_ID
                viewModelScope.launch {
                    try {
                        profileRepository?.setProfileLock(activeId, intent.config)
                        val updated = profileRepository?.getActiveProfile()
                        setState { copy(activeProfile = updated, showLockSetupDialog = false) }
                        sendEffect(SettingsEffect.ShowSnackbar("Profile lock updated."))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to update profile lock: ${e.message}") }
                    }
                }
            }
            is SettingsIntent.SwitchProfile -> {
                viewModelScope.launch {
                    try {
                        profileRepository?.setActiveProfile(intent.profileId)
                        val updated = profileRepository?.getActiveProfile()
                        setState { copy(activeProfile = updated) }
                        checkCliStatus()
                        sendEffect(SettingsEffect.ShowSnackbar("Switched to ${updated?.name}"))
                    } catch (e: Exception) {
                        setState { copy(error = "Failed to switch profile") }
                    }
                }
            }

            // ── Ollama & Local LLM Handlers ────────────────────────────────
            is SettingsIntent.RefreshOllama -> {
                viewModelScope.launch {
                    checkOllamaHealth(state.value.apiKeys.ollama)
                }
            }
            is SettingsIntent.UpdateOllamaEndpoint -> {
                val updatedKeys = state.value.apiKeys.copy(ollama = intent.endpoint)
                setState { copy(apiKeys = updatedKeys) }
                viewModelScope.launch {
                    settingsRepository.updateApiKeys(updatedKeys)
                    checkOllamaHealth(intent.endpoint)
                }
            }
            is SettingsIntent.PullOllamaModel -> {
                viewModelScope.launch {
                    setState { copy(isPullingOllamaModel = true, pullModelProgress = 0f, pullModelStatus = "Starting download...") }
                    val result = ollamaService.pullModel(
                        endpointUrl = state.value.apiKeys.ollama,
                        modelName = intent.modelName,
                        onProgress = { status, completed, total ->
                            val progress = if (total > 0) (completed.toFloat() / total).coerceIn(0f, 1f) else 0f
                            setState { copy(pullModelStatus = status, pullModelProgress = progress) }
                        }
                    )
                    if (result.isSuccess) {
                        sendEffect(SettingsEffect.ShowSnackbar("Model ${intent.modelName} downloaded successfully!"))
                        checkOllamaHealth(state.value.apiKeys.ollama)
                    } else {
                        setState { copy(error = "Failed to pull ${intent.modelName}: ${result.exceptionOrNull()?.message}") }
                    }
                    setState { copy(isPullingOllamaModel = false, pullModelProgress = 0f, pullModelStatus = "") }
                }
            }
        }
    }

    private suspend fun checkOllamaHealth(endpoint: String) {
        setState { copy(ollamaHealth = com.dialex.service.OllamaHealthStatus.Checking) }
        val health = ollamaService.checkHealth(endpoint)
        val models = if (health is com.dialex.service.OllamaHealthStatus.Online) {
            ollamaService.fetchInstalledModels(endpoint)
        } else {
            emptyList()
        }
        setState {
            copy(
                ollamaHealth = health,
                installedOllamaModels = models.toImmutableList()
            )
        }
    }

    private fun updateApiKeyMap(existing: ApiKeys, provider: Provider, key: String): ApiKeys = when (provider) {
        Provider.ANTHROPIC -> existing.copy(anthropic = key)
        Provider.OPENAI -> existing.copy(openai = key)
        Provider.GEMINI -> existing.copy(gemini = key)
        Provider.GROK -> existing.copy(grok = key)
        Provider.DEEPSEEK -> existing.copy(deepseek = key)
        Provider.MISTRAL -> existing.copy(mistral = key)
        Provider.OLLAMA -> existing.copy(ollama = key)
        Provider.CUSTOM -> existing
    }
}
