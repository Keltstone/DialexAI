package com.dialex.presentation.chat

import com.dialex.domain.repository.DiscussionRepository
import com.dialex.domain.repository.DiscussionUsage
import com.dialex.model.Agent
import com.dialex.model.DebateConfig
import com.dialex.model.DebateMessage
import com.dialex.model.DeliverableConfig
import com.dialex.model.DeliverableFormat
import com.dialex.model.Discussion
import com.dialex.model.DiscussionStatus
import com.dialex.model.Provider
import com.dialex.model.RoundMode
import com.dialex.model.defaultModel
import com.dialex.presentation.chat.components.effectiveRoundFor
import com.dialex.presentation.chat.components.resolveAllArtifacts
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class)
class ChatChronologyTest {

    private val testDispatcher = StandardTestDispatcher()

    @BeforeTest
    fun setUp() {
        Dispatchers.setMain(testDispatcher)
    }

    @AfterTest
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private class FakeDiscussionRepository(
        var currentDiscussion: Discussion
    ) : DiscussionRepository {
        var lastUpdatedDiscussion: Discussion? = null
        var resumeCallCount = 0

        override suspend fun getDiscussions(projectId: String?): List<Discussion> = listOf(currentDiscussion)
        override suspend fun getDiscussion(id: String): Discussion = currentDiscussion
        override suspend fun createDiscussion(projectId: String, name: String, config: DebateConfig): Discussion = currentDiscussion
        override suspend fun updateDiscussion(discussion: Discussion): Discussion {
            currentDiscussion = discussion
            lastUpdatedDiscussion = discussion
            return discussion
        }
        override suspend fun deleteDiscussion(id: String) {}
        override suspend fun duplicateDiscussion(id: String): Discussion = currentDiscussion
        override suspend fun startDiscussion(id: String) {}
        override suspend fun pauseDiscussion(id: String) {
            currentDiscussion = currentDiscussion.copy(status = DiscussionStatus.PAUSED)
        }
        override suspend fun stopDiscussion(id: String) {}
        override suspend fun resumeDiscussion(id: String) {
            resumeCallCount++
            currentDiscussion = currentDiscussion.copy(status = DiscussionStatus.PAUSED)
        }
        override fun streamDiscussion(id: String): Flow<Discussion> = emptyFlow()
        override suspend fun generateHandoffPrompt(id: String): String = ""
        override suspend fun generateDeliverableFormat(discussionId: String, format: DeliverableFormat): String = ""
        override suspend fun generateDiscussionTitle(id: String): String = "Generated Title"
        override suspend fun getUsage(id: String): DiscussionUsage = DiscussionUsage(0, 0, 0.0)
    }

    @Test
    fun testSendUserCommentOnCompletedDiscussionStartsFreshRoundAndResumes() = runTest(testDispatcher) {
        val agent1 = Agent(id = "agent_1", provider = Provider.ANTHROPIC, model = Provider.ANTHROPIC.defaultModel())
        val agent2 = Agent(id = "agent_2", provider = Provider.OPENAI, model = Provider.OPENAI.defaultModel())

        val initialDiscussion = Discussion(
            id = "disc_1",
            projectId = "proj_1",
            name = "Test Discussion",
            config = DebateConfig(
                topic = "Choose between Keltstone Systems and Illam Labs",
                primary = agent1,
                secondary = agent2,
                roundMode = RoundMode.FIXED,
                maxRounds = 1,
                deliverable = DeliverableConfig(format = DeliverableFormat.DECISION_SUMMARY)
            ),
            status = DiscussionStatus.COMPLETED,
            transcript = listOf(
                DebateMessage(seatId = "agent_1", agentId = Provider.ANTHROPIC, round = 1, content = "Keltstone Systems is great", timestampMs = 1000L),
                DebateMessage(seatId = "agent_2", agentId = Provider.OPENAI, round = 1, content = "AGREED: Keltstone Systems is optimal", timestampMs = 2000L)
            ),
            conclusion = "Final Decision: Keltstone Systems",
            deliverable = "Final Decision: Keltstone Systems"
        )

        val repo = FakeDiscussionRepository(initialDiscussion)
        val viewModel = ChatViewModel(
            discussionRepository = repo,
            discussionId = "disc_1",
            tokenBudget = 100_000
        )
        advanceUntilIdle()

        // User sends comment after discussion is COMPLETED
        viewModel.onIntent(ChatIntent.SendUserComment("Keltstone Systems is not available. Check suitability for Medhas and GanLabs"))
        testScheduler.runCurrent()

        val updatedDisc = repo.lastUpdatedDiscussion
        assertNotNull(updatedDisc, "Repository should have received updated discussion")

        // 1. Discussion must be set to RUNNING
        assertEquals(DiscussionStatus.RUNNING, updatedDisc.status)

        // 2. Conclusion, deliverable, and summary must be reset to null for the fresh deliberation
        assertNull(updatedDisc.conclusion)
        assertNull(updatedDisc.deliverable)
        assertNull(updatedDisc.summary)

        // 3. Round 1 artifacts must be snapshotted
        assertTrue(updatedDisc.artifacts.any { it.round == 1 || it.id.contains("_r1") }, "Round 1 artifacts must be preserved")

        // 4. User comment must be in Round 2 and marked isUserComment
        val lastMsg = updatedDisc.transcript.last()
        assertTrue(lastMsg.isUserComment)
        assertEquals(2, lastMsg.round)
        assertEquals("Keltstone Systems is not available. Check suitability for Medhas and GanLabs", lastMsg.content)

        // 5. Resume was invoked on the repository
        assertEquals(1, repo.resumeCallCount)
    }

    @Test
    fun testEffectiveRoundForAndChronologicalSorting() {
        val discussion = Discussion(
            id = "disc_chrono",
            projectId = "proj_1",
            name = "Chrono Discussion",
            config = DebateConfig(
                topic = "Topic",
                primary = Agent(id = "agent_1", provider = Provider.ANTHROPIC, model = Provider.ANTHROPIC.defaultModel()),
                secondary = Agent(id = "agent_2", provider = Provider.OPENAI, model = Provider.OPENAI.defaultModel())
            ),
            transcript = listOf(
                DebateMessage(seatId = "agent_1", agentId = Provider.ANTHROPIC, round = 1, content = "Round 1 Turn 1", timestampMs = 10000L),
                DebateMessage(seatId = "agent_2", agentId = Provider.OPENAI, round = 1, content = "Round 1 Turn 2", timestampMs = 20000L),
                DebateMessage(seatId = "observer", round = 2, content = "User comment in round 2", isUserComment = true, timestampMs = 30000L),
                DebateMessage(seatId = "agent_1", agentId = Provider.ANTHROPIC, round = 2, content = "Round 2 Turn 1", timestampMs = 40000L),
                DebateMessage(seatId = "agent_2", agentId = Provider.OPENAI, round = 2, content = "Round 2 Turn 2", timestampMs = 50000L)
            ),
            artifacts = listOf(
                com.dialex.model.DiscussionArtifact(id = "doc_1", name = "Requirements.pdf", type = "Reference Document", format = "pdf", content = "", timestampMs = 0L),
                com.dialex.model.DiscussionArtifact(id = "art_sum_r1", name = "Summary R1", type = "Summary", format = "md", content = "", timestampMs = 21000L),
                com.dialex.model.DiscussionArtifact(id = "art_tra_r1", name = "Transcript R1", type = "Transcript", format = "md", content = "", timestampMs = 21500L),
                com.dialex.model.DiscussionArtifact(id = "art_del_r2", name = "Deliverable R2", type = "Deliverable", format = "md", content = "", timestampMs = 52000L),
                com.dialex.model.DiscussionArtifact(id = "art_sum_r2", name = "Summary R2", type = "Summary", format = "md", content = "", timestampMs = 51000L)
            )
        )

        // Reference doc has null round (pre-debate)
        assertNull(discussion.effectiveRoundFor(discussion.artifacts[0]))

        // Round 1 artifacts map to round 1
        assertEquals(1, discussion.effectiveRoundFor(discussion.artifacts[1]))
        assertEquals(1, discussion.effectiveRoundFor(discussion.artifacts[2]))

        // Round 2 artifacts map to round 2
        assertEquals(2, discussion.effectiveRoundFor(discussion.artifacts[3]))
        assertEquals(2, discussion.effectiveRoundFor(discussion.artifacts[4]))

        // Check chronological sorting for Round 2: Summary (51000L) must come before Deliverable (52000L)
        val round2Artifacts = discussion.artifacts
            .filter { discussion.effectiveRoundFor(it) == 2 }
            .sortedWith(compareBy<com.dialex.model.DiscussionArtifact> { if (it.timestampMs > 0L) it.timestampMs else 0L }.thenBy { it.id })

        assertEquals("art_sum_r2", round2Artifacts[0].id)
        assertEquals("art_del_r2", round2Artifacts[1].id)
    }

    @Test
    fun testResolveAllArtifactsOnlyAutoGeneratesSummaryAndTranscript() {
        val discussion = Discussion(
            id = "disc_resolve",
            projectId = "proj_1",
            name = "Resolve Discussion",
            config = DebateConfig(
                topic = "Topic",
                primary = Agent(id = "agent_1", provider = Provider.ANTHROPIC, model = Provider.ANTHROPIC.defaultModel()),
                secondary = Agent(id = "agent_2", provider = Provider.OPENAI, model = Provider.OPENAI.defaultModel())
            ),
            transcript = listOf(
                DebateMessage(seatId = "agent_1", agentId = Provider.ANTHROPIC, round = 1, content = "Point 1", timestampMs = 1000L)
            ),
            summary = "Summary text",
            conclusion = "Conclusion text",
            deliverable = "Deliverable text"
        )

        val resolved = discussion.resolveAllArtifacts(snapshotRound = 1)
        val types = resolved.map { it.type }

        // Must generate Summary and Transcript
        assertTrue(types.contains("Summary"), "Should contain Summary")
        assertTrue(types.contains("Transcript"), "Should contain Transcript")

        // Must NOT automatically fabricate Deliverable or Executive Memorandum without on-demand request
        assertEquals(0, resolved.count { it.type.equals("Deliverable", ignoreCase = true) }, "Deliverable must not be auto-fabricated")
        assertEquals(0, resolved.count { it.name.contains("Executive Memorandum", ignoreCase = true) }, "Executive Memo must not be auto-fabricated")
    }

    @Test
    fun testFormatDateTimeOutputsDateAndFormattedTime() {
        val dtStr = com.dialex.util.formatDateTime(1726857300000L) // Sep 20 2024 (approx)
        assertTrue(dtStr.isNotEmpty(), "Date/time string should not be empty")
        assertTrue(dtStr.contains(":") && (dtStr.contains("AM") || dtStr.contains("PM")), "Should contain formatted time with AM/PM")
    }
}


