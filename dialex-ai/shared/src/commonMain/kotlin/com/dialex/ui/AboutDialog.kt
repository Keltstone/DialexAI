package com.dialex.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import com.dialex.theme.LocalCcColors

/**
 * Clean About dialog triggered from the OS application menu (About Dialex) or Settings.
 * Focuses purely on product identity, company attribution, and version info without
 * internal technology stack mentions.
 */
@Composable
fun AboutDialog(
    authorName: String = "Arun Electra",
    companyName: String = "Kolta Labs",
    onDismiss: () -> Unit,
    onOpenFullAbout: (() -> Unit)? = null,
    onOpenLicensingDedication: (() -> Unit)? = null
) {
    val cc = LocalCcColors.current

    Dialog(onDismissRequest = onDismiss) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            color = cc.panel,
            border = BorderStroke(1.dp, cc.border.copy(alpha = 0.5f)),
            modifier = Modifier.width(390.dp)
        ) {
            Box(modifier = Modifier.fillMaxWidth()) {
                // Top-right small close button
                IconButton(
                    onClick = onDismiss,
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(top = 10.dp, end = 10.dp)
                        .size(28.dp)
                ) {
                    Icon(
                        Icons.Filled.Close,
                        contentDescription = "Close",
                        tint = cc.textMuted,
                        modifier = Modifier.size(15.dp)
                    )
                }

                SelectionContainer {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 22.dp, bottom = 22.dp, start = 24.dp, end = 24.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                    DialexLogoView(
                        size = 58.dp,
                        modifier = Modifier.clip(RoundedCornerShape(13.dp))
                    )

                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text(
                            "Dialex AI",
                            style = MaterialTheme.typography.titleLarge.copy(
                                fontWeight = FontWeight.Bold,
                                fontSize = 21.sp
                            ),
                            color = cc.textPrimary
                        )
                        Text(
                            "Version 1.0.0",
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Normal
                            ),
                            color = cc.textMuted
                        )
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "Autonomous Multi-Agent Deliberation & Decision Intelligence Council",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, fontWeight = FontWeight.Medium, textAlign = TextAlign.Center),
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.height(2.dp))
                        Text(
                            "Local-first consensus engineering for mission-critical architectural and technical decisions.",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp, textAlign = TextAlign.Center),
                            color = cc.textMuted
                        )
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

                    Column(
                        modifier = Modifier.fillMaxWidth(),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("Organization", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp), color = cc.textMuted)
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                KoltaLabsLogoView(size = 14.dp)
                                Text(companyName, style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, fontWeight = FontWeight.SemiBold), color = cc.textPrimary)
                            }
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("Engine", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp), color = cc.textMuted)
                            Text("Dialex AI Go Orchestration Engine", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("License", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp), color = cc.textMuted)
                            Text("PolyForm Noncommercial 1.0.0", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                        }
                    }

                    if (onOpenFullAbout != null) {
                        OutlinedButton(
                            onClick = {
                                onDismiss()
                                onOpenFullAbout()
                            },
                            shape = RoundedCornerShape(8.dp),
                            modifier = Modifier.fillMaxWidth().height(32.dp),
                            contentPadding = PaddingValues(horizontal = 12.dp)
                        ) {
                            Text("View Full About & Legal Docs", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp), color = cc.textPrimary)
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

                    Row(
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Copyright © 2026 Kolta Labs. Free & Open Source.",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 10.5.sp),
                            color = cc.textMuted.copy(alpha = 0.6f)
                        )
                        if (onOpenLicensingDedication != null) {
                            Text(
                                "• Legal",
                                style = MaterialTheme.typography.bodySmall.copy(
                                    fontSize = 10.5.sp,
                                    color = cc.textMuted.copy(alpha = 0.75f)
                                ),
                                modifier = Modifier
                                    .clip(RoundedCornerShape(3.dp))
                                    .clickable {
                                        onDismiss()
                                        onOpenLicensingDedication()
                                    }
                                    .padding(horizontal = 2.dp)
                            )
                        }
                    }
                }
            }
            }
        }
    }
}
