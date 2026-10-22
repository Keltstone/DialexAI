package com.dialex.presentation.settings.personas

import com.dialex.domain.model.PersonaChatMessage
import com.dialex.model.PredefinedPersona
import com.dialex.model.Provider
import com.dialex.model.RunMode
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.persistentListOf
// Persona Builder Contract — State / Intent / Effect
data class PersonaBuilderState(
    /** The persona being edited — starts with defaults for new personas, or the loaded persona
     * when editing an existing one ([personaId] was non-null on navigation). */
    val draft: PredefinedPersona = PredefinedPersona(
        id = "",
        name = "",
        description = "",
    ),
    val isEditing: Boolean = false,
    val saveAsync: AsyncState<Unit> = AsyncState.Idle,
    val validationError: String? = null,

    // ── JSON Import Dialog State ──
    val isImportDialogOpen: Boolean = false,
    val importInputText: String = "",
    val importError: String? = null,

    // ── AI Persona Assistant Chat State ──
    val isChatDrawerOpen: Boolean = false,
    val isChatGenerating: Boolean = false,
    val chatError: String? = null,
    val chatMessages: ImmutableList<PersonaChatMessage> = persistentListOf(),
    val selectedProvider: Provider = Provider.ANTHROPIC,
    val selectedModel: String = "claude-sonnet-5",
    val selectedRunMode: RunMode = RunMode.API,
    val selectedCliCommand: String = "claude",
    val availableModels: ImmutableList<String> = persistentListOf(
        "claude-sonnet-5",
        "claude-opus-5",
        "claude-haiku-4-5-20251001",
        "gpt-5.6-sol",
        "gpt-5.6-terra",
        "gpt-5.5",
        "gemini-3.7-flash",
        "gemini-3.1-pro",
        "gemini-3.5-flash-lite",
        "grok-4-fast",
        "grok-4",
        "deepseek-chat",
        "deepseek-reasoner",
        "mistral-large-latest",
    ),

    // ── 8-Layer Cognitive DNA Studio ──
    val isDnaStudioOpen: Boolean = false,
)

sealed interface PersonaBuilderIntent {
    data class NameChanged(val name: String) : PersonaBuilderIntent
    data class CategoryChanged(val category: String) : PersonaBuilderIntent
    data class RoleChanged(val role: String) : PersonaBuilderIntent
    data class DescriptionChanged(val description: String) : PersonaBuilderIntent
    data class IconChanged(val icon: String) : PersonaBuilderIntent
    data class SystemPromptChanged(val prompt: String) : PersonaBuilderIntent
    data class RoleAndPersonaChanged(val value: String) : PersonaBuilderIntent
    data class CoreExpertiseChanged(val value: String) : PersonaBuilderIntent
    data class ToneAndVoiceChanged(val value: String) : PersonaBuilderIntent
    data class ObjectiveChanged(val value: String) : PersonaBuilderIntent
    data class PonytailToggled(val enabled: Boolean) : PersonaBuilderIntent
    data object AutoComposePrompt : PersonaBuilderIntent
    data object Save : PersonaBuilderIntent
    data object Discard : PersonaBuilderIntent

    // ── 8-Layer DNA Studio Intents ──
    data class ToggleDnaStudio(val open: Boolean? = null) : PersonaBuilderIntent
    data class ApplyDnaToDraft(val dna: com.dialex.domain.model.PersonaDNA, val compiledPrompt: String? = null) : PersonaBuilderIntent

    // ── JSON Import Intents ──
    data object OpenImportDialog : PersonaBuilderIntent
    data object DismissImportDialog : PersonaBuilderIntent
    data class ImportInputChanged(val text: String) : PersonaBuilderIntent
    data class ImportJson(val raw: String? = null) : PersonaBuilderIntent

    // ── AI Persona Assistant Chat Intents ──
    data class ToggleChatDrawer(val open: Boolean? = null) : PersonaBuilderIntent
    data class SelectProvider(val provider: Provider) : PersonaBuilderIntent
    data class SelectModel(val model: String) : PersonaBuilderIntent
    data class SelectRunMode(val runMode: RunMode) : PersonaBuilderIntent
    data class SelectCliCommand(val command: String) : PersonaBuilderIntent
    data class SendChatMessage(val text: String) : PersonaBuilderIntent
    data class ApplyPersonaFromChat(val persona: PredefinedPersona) : PersonaBuilderIntent
    data object InjectCurrentDraftToChat : PersonaBuilderIntent
    data object ClearChat : PersonaBuilderIntent
}

sealed interface PersonaBuilderEffect {
    data object NavigateBack : PersonaBuilderEffect
    data class ShowSnackbar(val message: String) : PersonaBuilderEffect
}

