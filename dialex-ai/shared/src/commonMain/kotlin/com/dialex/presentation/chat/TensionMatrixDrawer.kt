package com.dialex.presentation.chat

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.ElectricBolt
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Scale
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.dialex.domain.model.AgentAssertion
import com.dialex.domain.model.TensionFilter
import com.dialex.domain.model.TensionPair
import com.dialex.domain.model.TensionStatus
import com.dialex.theme.LocalCcColors
import kotlinx.collections.immutable.ImmutableList

@Composable
fun TensionMatrixDrawer(
    tensions: ImmutableList<TensionPair>,
    filter: TensionFilter,
    onFilterSelect: (TensionFilter) -> Unit,
    onDismiss: () -> Unit,
) {
    val cc = LocalCcColors.current

    val openCount = tensions.count { it.status == TensionStatus.OPEN || it.status == TensionStatus.EXPLORED }
    val resolvedCount = tensions.count { it.status == TensionStatus.RESOLVED }
    val tradeOffCount = tensions.count { it.status == TensionStatus.ACCEPTED_TRADE_OFF }

    val filteredTensions = when (filter) {
        TensionFilter.ALL -> tensions
        TensionFilter.OPEN_ONLY -> tensions.filter { it.status == TensionStatus.OPEN || it.status == TensionStatus.EXPLORED }
        TensionFilter.RESOLVED_ONLY -> tensions.filter { it.status == TensionStatus.RESOLVED }
        TensionFilter.TRADE_OFFS -> tensions.filter { it.status == TensionStatus.ACCEPTED_TRADE_OFF }
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        BoxWithConstraints(modifier = Modifier.fillMaxSize()) {
            val isCompact = maxWidth < 640.dp
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = if (isCompact) Alignment.BottomCenter else Alignment.Center
            ) {
                Surface(
                    modifier = Modifier
                        .fillMaxWidth(if (isCompact) 1f else 0.92f)
                        .fillMaxHeight(if (isCompact) 0.92f else 0.88f)
                        .clip(
                            if (isCompact) RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp)
                            else RoundedCornerShape(16.dp)
                        ),
                    color = cc.bg,
                    border = BorderStroke(1.dp, cc.border),
                    shadowElevation = 24.dp
                ) {
                    Column(modifier = Modifier.fillMaxSize()) {
                        if (isCompact) {
                            Box(
                                modifier = Modifier
                                    .padding(top = 10.dp)
                                    .size(36.dp, 4.dp)
                                    .clip(RoundedCornerShape(2.dp))
                                    .background(cc.border.copy(alpha = 0.8f))
                                    .align(Alignment.CenterHorizontally)
                            )
                        }
                // Header
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 24.dp, vertical = 18.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            modifier = Modifier
                                .size(40.dp)
                                .clip(RoundedCornerShape(10.dp))
                                .background(cc.accent.copy(alpha = 0.14f)),
                            contentAlignment = Alignment.Center
                        ) {
                            Icon(
                                imageVector = Icons.Outlined.ElectricBolt,
                                contentDescription = "Tension Engine",
                                tint = cc.accent,
                                modifier = Modifier.size(22.dp)
                            )
                        }
                        Spacer(Modifier.width(14.dp))
                        Column {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    text = "Paraconsistent Tension Matrix",
                                    color = cc.textPrimary,
                                    fontSize = 18.sp,
                                    fontWeight = FontWeight.Bold
                                )
                                Spacer(Modifier.width(8.dp))
                                if (openCount > 0) {
                                    Box(
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(6.dp))
                                            .background(Color(0xFFFEF3C7))
                                            .padding(horizontal = 7.dp, vertical = 2.dp)
                                    ) {
                                        Text(
                                            text = "$openCount Active",
                                            color = Color(0xFFD97706),
                                            fontSize = 11.sp,
                                            fontWeight = FontWeight.Bold
                                        )
                                    }
                                } else if (tensions.isNotEmpty()) {
                                    Box(
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(6.dp))
                                            .background(Color(0xFFD1FAE5))
                                            .padding(horizontal = 7.dp, vertical = 2.dp)
                                    ) {
                                        Text(
                                            text = "All Resolved",
                                            color = Color(0xFF059669),
                                            fontSize = 11.sp,
                                            fontWeight = FontWeight.Bold
                                        )
                                    }
                                }
                            }
                            Text(
                                text = "Isolating orthogonal contradictions (C ∧ ¬C ⊬ ⊥) • Gatekeeper prevents superficial consensus",
                                color = cc.textMuted,
                                fontSize = 12.sp
                            )
                        }
                    }

                    IconButton(onClick = onDismiss) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = "Close",
                            tint = cc.textMuted
                        )
                    }
                }

                HorizontalDivider(color = cc.border)

                // Filter Tab Bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 24.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    TensionFilterTab(
                        label = "All (${tensions.size})",
                        isSelected = filter == TensionFilter.ALL,
                        onClick = { onFilterSelect(TensionFilter.ALL) }
                    )
                    TensionFilterTab(
                        label = "Open / Active ($openCount)",
                        badgeColor = if (openCount > 0) Color(0xFFD97706) else null,
                        isSelected = filter == TensionFilter.OPEN_ONLY,
                        onClick = { onFilterSelect(TensionFilter.OPEN_ONLY) }
                    )
                    TensionFilterTab(
                        label = "Resolved ($resolvedCount)",
                        badgeColor = if (resolvedCount > 0) Color(0xFF059669) else null,
                        isSelected = filter == TensionFilter.RESOLVED_ONLY,
                        onClick = { onFilterSelect(TensionFilter.RESOLVED_ONLY) }
                    )
                    TensionFilterTab(
                        label = "Trade-offs ($tradeOffCount)",
                        badgeColor = if (tradeOffCount > 0) Color(0xFF7C3AED) else null,
                        isSelected = filter == TensionFilter.TRADE_OFFS,
                        onClick = { onFilterSelect(TensionFilter.TRADE_OFFS) }
                    )
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.5f))

                // List or Empty View
                if (filteredTensions.isEmpty()) {
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(32.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(54.dp)
                                    .clip(CircleShape)
                                    .background(cc.panelAlt),
                                contentAlignment = Alignment.Center
                            ) {
                                Icon(
                                    imageVector = Icons.Outlined.Info,
                                    contentDescription = null,
                                    tint = cc.textMuted,
                                    modifier = Modifier.size(28.dp)
                                )
                            }
                            Text(
                                text = "No Contradictions In This View",
                                color = cc.textPrimary,
                                fontSize = 15.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            Text(
                                text = if (tensions.isEmpty()) {
                                    "As council participants articulate conflicting architectural requirements, the Paraconsistent Logic engine will automatically extract and index dialectic tension pairs here."
                                } else {
                                    "No tension pairs match the selected filter tab."
                                },
                                color = cc.textMuted,
                                fontSize = 13.sp,
                                modifier = Modifier.fillMaxWidth(0.6f),
                                lineHeight = 18.sp
                            )
                        }
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(horizontal = 24.dp, vertical = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp)
                    ) {
                        items(filteredTensions, key = { it.id }) { tension ->
                            TensionCard(tension = tension)
                        }
                    }
                }
            }
        }
    }
}
}
}

@Composable
private fun TensionFilterTab(
    label: String,
    badgeColor: Color? = null,
    isSelected: Boolean,
    onClick: () -> Unit
) {
    val cc = LocalCcColors.current
    val bgColor = if (isSelected) cc.accent.copy(alpha = 0.16f) else cc.panelAlt
    val textColor = if (isSelected) cc.accent else cc.textMuted
    val borderColor = if (isSelected) cc.accent.copy(alpha = 0.4f) else cc.border

    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(8.dp))
            .background(bgColor)
            .border(1.dp, borderColor, RoundedCornerShape(8.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        if (badgeColor != null) {
            Box(
                modifier = Modifier
                    .size(6.dp)
                    .clip(CircleShape)
                    .background(badgeColor)
            )
        }
        Text(
            text = label,
            color = textColor,
            fontSize = 12.sp,
            fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Medium
        )
    }
}

@Composable
private fun TensionCard(tension: TensionPair) {
    val cc = LocalCcColors.current

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(cc.panel)
            .border(1.dp, cc.border, RoundedCornerShape(12.dp))
            .padding(18.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        TensionCardHeader(
            underlyingConflict = tension.underlyingConflict,
            detectedInRound = tension.detectedInRound,
            status = tension.status
        )

        TensionDialecticSplit(
            thesis = tension.thesis,
            antithesis = tension.antithesis
        )

        TensionStatusCallout(tension = tension)
    }
}

@Composable
private fun TensionCardHeader(
    underlyingConflict: String,
    detectedInRound: Int,
    status: TensionStatus
) {
    val cc = LocalCcColors.current

    val statusColor = when (status) {
        TensionStatus.OPEN -> Color(0xFFD97706)
        TensionStatus.EXPLORED -> Color(0xFF2563EB)
        TensionStatus.RESOLVED -> Color(0xFF059669)
        TensionStatus.ACCEPTED_TRADE_OFF -> Color(0xFF7C3AED)
    }

    val statusBg = when (status) {
        TensionStatus.OPEN -> Color(0xFFFEF3C7)
        TensionStatus.EXPLORED -> Color(0xFFDBEAFE)
        TensionStatus.RESOLVED -> Color(0xFFD1FAE5)
        TensionStatus.ACCEPTED_TRADE_OFF -> Color(0xFFEDE9FE)
    }

    val statusLabel = when (status) {
        TensionStatus.OPEN -> "OPEN CONTRADICTION"
        TensionStatus.EXPLORED -> "EXPLORED"
        TensionStatus.RESOLVED -> "RESOLVED & SYNTHESIZED"
        TensionStatus.ACCEPTED_TRADE_OFF -> "ACCEPTED TRADE-OFF"
    }

    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .background(cc.accent.copy(alpha = 0.12f))
                    .padding(horizontal = 8.dp, vertical = 4.dp)
            ) {
                Text(
                    text = underlyingConflict,
                    color = cc.accent,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Bold
                )
            }
            Spacer(Modifier.width(10.dp))
            Text(
                text = "Detected Round $detectedInRound",
                color = cc.textMuted,
                fontSize = 11.sp
            )
        }

        Box(
            modifier = Modifier
                .clip(RoundedCornerShape(6.dp))
                .background(statusBg)
                .padding(horizontal = 9.dp, vertical = 4.dp)
        ) {
            Text(
                text = statusLabel,
                color = statusColor,
                fontSize = 10.sp,
                fontWeight = FontWeight.Bold,
                letterSpacing = 0.5.sp
            )
        }
    }
}

@Composable
private fun TensionDialecticSplit(
    thesis: AgentAssertion,
    antithesis: AgentAssertion
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        DialecticAssertionCard(
            label = "THESIS • ${thesis.authorDisplayName}",
            assertion = thesis,
            modifier = Modifier.weight(1f)
        )
        DialecticAssertionCard(
            label = "ANTITHESIS • ${antithesis.authorDisplayName}",
            assertion = antithesis,
            modifier = Modifier.weight(1f)
        )
    }
}

@Composable
private fun DialecticAssertionCard(
    label: String,
    assertion: AgentAssertion,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(cc.panelAlt)
            .border(1.dp, cc.border.copy(alpha = 0.5f), RoundedCornerShape(8.dp))
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = label,
                color = cc.textPrimary,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
                letterSpacing = 0.3.sp
            )
            Text(
                text = assertion.provider.name,
                color = cc.textMuted,
                fontSize = 10.sp,
                fontFamily = FontFamily.Monospace
            )
        }
        Text(
            text = assertion.statement,
            color = cc.textPrimary.copy(alpha = 0.92f),
            fontSize = 13.sp,
            lineHeight = 18.sp
        )
        if (!assertion.quote.isNullOrBlank()) {
            Text(
                text = "\"${assertion.quote}\"",
                color = cc.textMuted,
                fontSize = 11.sp,
                fontStyle = FontStyle.Italic,
                lineHeight = 15.sp
            )
        }
    }
}

@Composable
private fun TensionStatusCallout(tension: TensionPair) {
    when (tension.status) {
        TensionStatus.RESOLVED -> ResolvedCallout(
            resolvedInRound = tension.resolvedInRound ?: (tension.detectedInRound + 1),
            synthesis = tension.synthesis
        )
        TensionStatus.ACCEPTED_TRADE_OFF -> TradeOffCallout(
            tradeOffRationale = tension.tradeOffRationale
        )
        TensionStatus.OPEN, TensionStatus.EXPLORED -> DeadlockCallout()
    }
}

@Composable
private fun ResolvedCallout(resolvedInRound: Int, synthesis: String?) {
    val cc = LocalCcColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(Color(0xFFD1FAE5).copy(alpha = 0.35f))
            .border(1.dp, Color(0xFF059669).copy(alpha = 0.4f), RoundedCornerShape(8.dp))
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Icon(
            imageVector = Icons.Outlined.CheckCircle,
            contentDescription = null,
            tint = Color(0xFF059669),
            modifier = Modifier.size(18.dp)
        )
        Column {
            Text(
                text = "Synthesized in Round $resolvedInRound",
                color = Color(0xFF059669),
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold
            )
            Text(
                text = synthesis ?: "Contradiction resolved by convergence on hybrid architectural pattern.",
                color = cc.textPrimary.copy(alpha = 0.9f),
                fontSize = 12.sp,
                lineHeight = 16.sp
            )
        }
    }
}

@Composable
private fun TradeOffCallout(tradeOffRationale: String?) {
    val cc = LocalCcColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(Color(0xFFEDE9FE).copy(alpha = 0.35f))
            .border(1.dp, Color(0xFF7C3AED).copy(alpha = 0.4f), RoundedCornerShape(8.dp))
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Icon(
            imageVector = Icons.Outlined.Scale,
            contentDescription = null,
            tint = Color(0xFF7C3AED),
            modifier = Modifier.size(18.dp)
        )
        Column {
            Text(
                text = "Accepted Architectural Trade-off",
                color = Color(0xFF7C3AED),
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold
            )
            Text(
                text = tradeOffRationale ?: "The council ratified this trade-off as an irreducible Pareto constraint.",
                color = cc.textPrimary.copy(alpha = 0.9f),
                fontSize = 12.sp,
                lineHeight = 16.sp
            )
        }
    }
}

@Composable
private fun DeadlockCallout() {
    val cc = LocalCcColors.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(Color(0xFFFEF3C7).copy(alpha = 0.35f))
            .border(1.dp, Color(0xFFD97706).copy(alpha = 0.4f), RoundedCornerShape(8.dp))
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Icon(
            imageVector = Icons.Outlined.Warning,
            contentDescription = null,
            tint = Color(0xFFD97706),
            modifier = Modifier.size(18.dp)
        )
        Column {
            Text(
                text = "Active Dialectic Deadlock",
                color = Color(0xFFD97706),
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold
            )
            Text(
                text = "Blocks consensus certification. Opposing models must formulate an architectural synthesis or formally log an accepted trade-off.",
                color = cc.textPrimary.copy(alpha = 0.9f),
                fontSize = 12.sp,
                lineHeight = 16.sp
            )
        }
    }
}
