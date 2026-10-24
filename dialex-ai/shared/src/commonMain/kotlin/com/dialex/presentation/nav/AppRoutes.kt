package com.dialex.presentation.nav

import com.dialex.domain.model.DiscussionMode
import com.dialex.presentation.settings.SettingsTab
import kotlinx.serialization.Serializable

@Serializable
sealed interface AppRoute

/** Master workspace sidebar view for compact single-pane layout. */
@Serializable
data object Workspace : AppRoute

/** Shown before the engine connection is established. */
@Serializable
data object Connect : AppRoute

/**
 * Discussion setup screen — configure topic, agents, personas, modifiers, round mode.
 * [discussionId] is null for a brand-new discussion (not yet created on the server).
 */
@Serializable
data class Setup(
    val discussionId: String? = null,
    val initialProjectId: String? = null,
    val copyFromDiscussionId: String? = null,
    val initialMode: DiscussionMode = DiscussionMode.COUNCIL
) : AppRoute

/** Live debate chat view. The discussion must exist and be non-DRAFT. */
@Serializable
data class Chat(val discussionId: String) : AppRoute

/** Global settings (Appearance, AI Agents Hub, Master Instructions, Compaction, Personas, Limits, Connection). */
@Serializable
data class Settings(val initialTab: SettingsTab = SettingsTab.Appearance) : AppRoute

/** Persona Builder — create or edit a [PredefinedPersona]. Null means a new persona. */
@Serializable
data class PersonaBuilder(val personaId: String? = null) : AppRoute

/** Self-organizing Knowledge Graph visualizer and explorer. */
@Serializable
data class Graph(val projectId: String) : AppRoute

/** Deliberation Arena & Null Hypothesis Benchmark Suite. */
@Serializable
data object BenchmarkArena : AppRoute

/** Subtle statutory licensing dedication, third-party attribution ledger, and disclaimers. */
@Serializable
data object LicensingDedication : AppRoute
