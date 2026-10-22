package com.dialex.presentation.settings.personas

import androidx.lifecycle.viewModelScope
import com.dialex.domain.model.PersonaChatMessage
import com.dialex.domain.model.PersonaChatMessageDto
import com.dialex.domain.model.PersonaChatRequest
import com.dialex.domain.repository.PersonaRepository
import com.dialex.model.PredefinedPersona
import com.dialex.model.Provider
import com.dialex.model.RunMode
import com.dialex.model.SystemPersonas
import com.dialex.model.buildComposedSystemPrompt
import com.dialex.presentation.base.MviViewModel
import kolt.utils.state.AsyncState
import kotlinx.collections.immutable.persistentListOf
import kotlinx.collections.immutable.toPersistentList
import kotlinx.coroutines.launch
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.random.Random

class PersonaBuilderViewModel(
    private val personaRepository: PersonaRepository,
    private val personaId: String? = null,
) : MviViewModel<PersonaBuilderState, PersonaBuilderIntent, PersonaBuilderEffect>(
    PersonaBuilderState(isEditing = personaId != null)
) {
    private val jsonSerializer = Json {
        ignoreUnknownKeys = true
        isLenient = true
        coerceInputValues = true
        encodeDefaults = true
        prettyPrint = true
    }

    init {
        if (personaId != null) {
            viewModelScope.launch {
                try {
                    val all = personaRepository.getPersonas()
                    val target = (all + SystemPersonas).firstOrNull { it.id == personaId }
                    if (target != null) {
                        setState {
                            copy(
                                draft = if (target.isSystem) {
                                    target.copy(
                                        id = "",
                                        name = "${target.name} (Custom)",
                                        isSystem = false
                                    )
                                } else {
                                    target
                                }
                            )
                        }
                    }
                } catch (e: Exception) {
                    setState { copy(validationError = e.message ?: "Failed to load persona") }
                }
            }
        }
    }

    override fun onIntent(intent: PersonaBuilderIntent) {
        when (intent) {
            is PersonaBuilderIntent.NameChanged -> setState { copy(draft = draft.copy(name = intent.name)) }
            is PersonaBuilderIntent.CategoryChanged -> setState { copy(draft = draft.copy(category = intent.category)) }
            is PersonaBuilderIntent.RoleChanged -> setState { copy(draft = draft.copy(role = intent.role)) }
            is PersonaBuilderIntent.DescriptionChanged -> setState { copy(draft = draft.copy(description = intent.description)) }
            is PersonaBuilderIntent.IconChanged -> setState { copy(draft = draft.copy(icon = intent.icon)) }
            is PersonaBuilderIntent.SystemPromptChanged -> setState { copy(draft = draft.copy(systemPrompt = intent.prompt)) }
            is PersonaBuilderIntent.RoleAndPersonaChanged -> setState { copy(draft = draft.copy(roleAndPersona = intent.value)) }
            is PersonaBuilderIntent.CoreExpertiseChanged -> setState { copy(draft = draft.copy(coreExpertise = intent.value)) }
            is PersonaBuilderIntent.ToneAndVoiceChanged -> setState { copy(draft = draft.copy(toneAndVoice = intent.value)) }
            is PersonaBuilderIntent.ObjectiveChanged -> setState { copy(draft = draft.copy(objective = intent.value)) }
            is PersonaBuilderIntent.PonytailToggled -> setState { copy(draft = draft.copy(ponytail = intent.enabled)) }
            is PersonaBuilderIntent.AutoComposePrompt -> {
                val composed = buildComposedSystemPrompt(
                    roleAndPersona = state.value.draft.roleAndPersona,
                    coreExpertise = state.value.draft.coreExpertise,
                    toneAndVoice = state.value.draft.toneAndVoice,
                    objective = state.value.draft.objective,
                    systemContextPrompt = state.value.draft.systemPrompt
                )
                setState { copy(draft = draft.copy(systemPrompt = composed)) }
            }

            // ── JSON Import ────────────────────────────────────────────────────────
            is PersonaBuilderIntent.OpenImportDialog -> {
                setState { copy(isImportDialogOpen = true, importError = null) }
            }
            is PersonaBuilderIntent.DismissImportDialog -> {
                setState { copy(isImportDialogOpen = false, importError = null) }
            }
            is PersonaBuilderIntent.ImportInputChanged -> {
                setState { copy(importInputText = intent.text, importError = null) }
            }
            is PersonaBuilderIntent.ImportJson -> {
                val rawToParse = intent.raw ?: state.value.importInputText
                parseAndApplyJson(rawToParse)
            }

            // ── AI Persona Assistant Chat ──────────────────────────────────────────
            is PersonaBuilderIntent.ToggleChatDrawer -> {
                setState { copy(isChatDrawerOpen = intent.open ?: !isChatDrawerOpen) }
            }
            is PersonaBuilderIntent.SelectProvider -> {
                val newModels = modelsFor(intent.provider)
                val defaultModel = defaultModelFor(intent.provider)
                setState {
                    copy(
                        selectedProvider = intent.provider,
                        selectedModel = defaultModel,
                        availableModels = newModels.toPersistentList()
                    )
                }
            }
            is PersonaBuilderIntent.SelectModel -> {
                setState { copy(selectedModel = intent.model) }
            }
            is PersonaBuilderIntent.SelectRunMode -> {
                setState { copy(selectedRunMode = intent.runMode) }
            }
            is PersonaBuilderIntent.SelectCliCommand -> {
                setState { copy(selectedCliCommand = intent.command) }
            }
            is PersonaBuilderIntent.SendChatMessage -> {
                sendUserChatMessage(intent.text)
            }
            is PersonaBuilderIntent.ApplyPersonaFromChat -> {
                setState {
                    copy(
                        draft = intent.persona.copy(
                            id = if (draft.id.isNotBlank()) draft.id else intent.persona.id,
                            isSystem = false
                        )
                    )
                }
                viewModelScope.launch {
                    sendEffect(PersonaBuilderEffect.ShowSnackbar("Loaded '${intent.persona.name}' into editor form!"))
                }
            }
            is PersonaBuilderIntent.InjectCurrentDraftToChat -> {
                injectCurrentDraftIntoChat()
            }
            is PersonaBuilderIntent.ClearChat -> {
                setState { copy(chatMessages = persistentListOf(), chatError = null) }
            }

            // ── Save & Discard ─────────────────────────────────────────────────────
            is PersonaBuilderIntent.Save -> {
                val currentDraft = state.value.draft
                if (currentDraft.name.isBlank()) {
                    setState { copy(validationError = "Persona name is required") }
                    return
                }
                val effectivePrompt = if (currentDraft.systemPrompt.isNotBlank()) {
                    currentDraft.systemPrompt
                } else {
                    buildComposedSystemPrompt(
                        roleAndPersona = currentDraft.roleAndPersona,
                        coreExpertise = currentDraft.coreExpertise,
                        toneAndVoice = currentDraft.toneAndVoice,
                        objective = currentDraft.objective
                    )
                }
                if (effectivePrompt.isBlank()) {
                    setState { copy(validationError = "System context/prompt or persona attributes are required") }
                    return
                }
                setState { copy(saveAsync = AsyncState.Loading, validationError = null) }
                viewModelScope.launch {
                    try {
                        val effectiveRole = currentDraft.role.ifBlank {
                            currentDraft.roleAndPersona.lines().firstOrNull { it.isNotBlank() }?.take(50).orEmpty()
                        }
                        val baseDraft = currentDraft.copy(
                            systemPrompt = effectivePrompt,
                            role = effectiveRole,
                            isSystem = false
                        )
                        val toSave = if (baseDraft.id.isBlank()) {
                            val genId = "custom_" + Random.nextLong(100000, 999999).toString(36) + "_" + Random.nextInt(1000, 9999)
                            baseDraft.copy(id = genId)
                        } else {
                            baseDraft
                        }
                        personaRepository.createPersona(toSave)
                        setState { copy(saveAsync = AsyncState.Success(Unit)) }
                        sendEffect(PersonaBuilderEffect.NavigateBack)
                    } catch (e: Exception) {
                        setState { copy(saveAsync = AsyncState.Error(e), validationError = e.message) }
                    }
                }
            }
            is PersonaBuilderIntent.ToggleDnaStudio -> {
                setState { copy(isDnaStudioOpen = intent.open ?: !isDnaStudioOpen) }
            }
            is PersonaBuilderIntent.ApplyDnaToDraft -> {
                val currentDraft = state.value.draft
                val updatedPrompt = intent.compiledPrompt?.takeIf { it.isNotBlank() } ?: currentDraft.systemPrompt
                setState {
                    copy(
                        draft = currentDraft.copy(
                            dna = intent.dna,
                            systemPrompt = updatedPrompt
                        ),
                        isDnaStudioOpen = false
                    )
                }
                sendEffect(PersonaBuilderEffect.ShowSnackbar("8-Layer Cognitive DNA applied to '${currentDraft.name.ifBlank { "persona" }}'"))
            }
            is PersonaBuilderIntent.Discard -> {
                sendEffect(PersonaBuilderEffect.NavigateBack)
            }
        }
    }

    private fun parseAndApplyJson(rawText: String) {
        val cleaned = cleanJsonContent(rawText)
        if (cleaned.isBlank()) {
            setState { copy(importError = "Please enter or paste valid JSON content.") }
            return
        }
        try {
            val parsedPersona: PredefinedPersona = parsePersonaFromJsonString(cleaned)

            if (parsedPersona.name.isBlank() && parsedPersona.systemPrompt.isBlank()) {
                setState { copy(importError = "Parsed persona must have at least a name or system prompt.") }
                return
            }

            val effectivePrompt = if (parsedPersona.systemPrompt.isNotBlank()) {
                parsedPersona.systemPrompt
            } else {
                buildComposedSystemPrompt(
                    roleAndPersona = parsedPersona.roleAndPersona,
                    coreExpertise = parsedPersona.coreExpertise,
                    toneAndVoice = parsedPersona.toneAndVoice,
                    objective = parsedPersona.objective
                )
            }

            val populated = parsedPersona.copy(
                id = if (state.value.draft.id.isNotBlank()) state.value.draft.id else parsedPersona.id,
                systemPrompt = effectivePrompt,
                isSystem = false
            )

            setState {
                copy(
                    draft = populated,
                    isImportDialogOpen = false,
                    importInputText = "",
                    importError = null,
                    validationError = null
                )
            }
            viewModelScope.launch {
                sendEffect(PersonaBuilderEffect.ShowSnackbar("Loaded persona '${populated.name}' from JSON!"))
            }
        } catch (e: Exception) {
            setState { copy(importError = "Failed to parse JSON: ${e.message ?: "Invalid persona schema"}") }
        }
    }

    private fun parsePersonaFromJsonString(jsonStr: String): PredefinedPersona {
        val element = jsonSerializer.parseToJsonElement(jsonStr)
        val jsonObject = when (element) {
            is JsonArray -> element.firstOrNull()?.jsonObject ?: throw IllegalArgumentException("JSON array is empty")
            is JsonObject -> element
            else -> throw IllegalArgumentException("Expected JSON object or array")
        }

        val map = jsonObject.toMutableMap()
        if (!map.containsKey("id") || map["id"]?.jsonPrimitive?.contentOrNull.isNullOrBlank()) {
            map["id"] = JsonPrimitive("custom_" + Random.nextInt(10000, 99999))
        }
        if (!map.containsKey("name") || map["name"]?.jsonPrimitive?.contentOrNull.isNullOrBlank()) {
            val roleName = map["role"]?.jsonPrimitive?.contentOrNull
            map["name"] = JsonPrimitive(roleName?.ifBlank { "Custom Persona" } ?: "Custom Persona")
        }
        return jsonSerializer.decodeFromJsonElement(JsonObject(map))
    }

    private fun sendUserChatMessage(text: String) {
        val query = text.trim()
        if (query.isBlank()) return

        val userMessage = PersonaChatMessage(
            id = "msg_${Random.nextInt(100000, 999999)}",
            role = "user",
            content = query,
            timestamp = System.currentTimeMillis()
        )

        val updatedMessages = (state.value.chatMessages + userMessage).toPersistentList()
        setState {
            copy(
                chatMessages = updatedMessages,
                isChatGenerating = true,
                chatError = null
            )
        }

        viewModelScope.launch {
            try {
                val request = PersonaChatRequest(
                    provider = state.value.selectedProvider,
                    model = state.value.selectedModel,
                    runMode = state.value.selectedRunMode,
                    cliCommand = state.value.selectedCliCommand,
                    messages = updatedMessages.map { PersonaChatMessageDto(role = it.role, content = it.content) },
                    currentDraft = state.value.draft
                )

                val response = personaRepository.chatPersona(request)

                // If response didn't auto-parse persona, attempt client-side extraction from code blocks
                val extractedPersona = response.parsedPersona ?: extractPersonaFromText(response.reply)

                val assistantMessage = PersonaChatMessage(
                    id = "msg_${Random.nextInt(100000, 999999)}",
                    role = "assistant",
                    content = response.reply,
                    parsedPersona = extractedPersona,
                    timestamp = System.currentTimeMillis()
                )

                setState {
                    copy(
                        chatMessages = (chatMessages + assistantMessage).toPersistentList(),
                        isChatGenerating = false,
                        chatError = null
                    )
                }
            } catch (e: Exception) {
                setState {
                    copy(
                        isChatGenerating = false,
                        chatError = e.message ?: "Failed to generate AI response"
                    )
                }
            }
        }
    }

    private fun injectCurrentDraftIntoChat() {
        val currentDraft = state.value.draft
        val draftJson = jsonSerializer.encodeToString(currentDraft)
        val prompt = "Here is my current persona draft:\n\n```json\n$draftJson\n```\n" +
            "Please review this persona, suggest improvements, and provide a refined version with sharp tone and distinct expertise."
        sendUserChatMessage(prompt)
    }

    private fun extractPersonaFromText(text: String): PredefinedPersona? {
        val cleaned = cleanJsonContent(text)
        if (cleaned.isBlank()) return null
        return try {
            parsePersonaFromJsonString(cleaned)
        } catch (e: Exception) {
            null
        }
    }

    private fun cleanJsonContent(input: String): String {
        var text = input.trim()
        val firstFence = text.indexOf("```")
        if (firstFence != -1) {
            val afterFirstFence = text.substring(firstFence + 3).trimStart()
            val withoutLang = if (afterFirstFence.startsWith("json", ignoreCase = true)) {
                afterFirstFence.substring(4).trimStart()
            } else afterFirstFence
            val secondFence = withoutLang.lastIndexOf("```")
            if (secondFence != -1) {
                text = withoutLang.substring(0, secondFence).trim()
            } else {
                text = withoutLang.trim()
            }
        }

        val firstBrace = text.indexOf('{')
        val firstBracket = text.indexOf('[')
        if (firstBrace != -1 && (firstBracket == -1 || firstBrace < firstBracket)) {
            val lastBrace = text.lastIndexOf('}')
            if (lastBrace > firstBrace) {
                return text.substring(firstBrace, lastBrace + 1).trim()
            }
        } else if (firstBracket != -1) {
            val lastBracket = text.lastIndexOf(']')
            if (lastBracket > firstBracket) {
                return text.substring(firstBracket, lastBracket + 1).trim()
            }
        }
        return text.trim()
    }

    private fun defaultModelFor(provider: Provider): String = when (provider) {
        Provider.ANTHROPIC -> "claude-sonnet-5"
        Provider.OPENAI -> "gpt-5.6-sol"
        Provider.GEMINI -> "gemini-3.7-flash"
        Provider.GROK -> "grok-4-fast"
        Provider.DEEPSEEK -> "deepseek-chat"
        Provider.MISTRAL -> "mistral-large-latest"
        Provider.OLLAMA -> "llama3.2"
        Provider.CUSTOM -> "custom"
    }

    private fun modelsFor(provider: Provider): List<String> = when (provider) {
        Provider.ANTHROPIC -> listOf("claude-sonnet-5", "claude-opus-5", "claude-haiku-4-5-20251001")
        Provider.OPENAI -> listOf("gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.5", "gpt-5.4-mini")
        Provider.GEMINI -> listOf("gemini-3.7-flash", "gemini-3.1-pro", "gemini-3.5-flash-lite")
        Provider.GROK -> listOf("grok-4-fast", "grok-4", "grok-3", "grok-3-mini")
        Provider.DEEPSEEK -> listOf("deepseek-chat", "deepseek-reasoner")
        Provider.MISTRAL -> listOf("mistral-large-latest", "mistral-medium-latest", "mistral-small-latest")
        Provider.OLLAMA -> listOf("llama3.2", "mistral", "qwen2.5", "deepseek-r1")
        Provider.CUSTOM -> listOf("custom")
    }
}
