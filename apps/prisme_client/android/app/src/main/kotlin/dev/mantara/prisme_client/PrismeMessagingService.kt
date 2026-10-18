package dev.mantara.prisme_client

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.Uri
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

class PrismeMessagingService : FirebaseMessagingService() {
    override fun onMessageReceived(message: RemoteMessage) {
        val prefs = getSharedPreferences("prisme_push", MODE_PRIVATE)
        if (!prefs.getBoolean("enabled", false)) return
        val data = message.data
        val id = data["incident_id"] ?: return
        if (!id.matches(Regex("[a-f0-9]{64}"))) return
        val event = data["event_id"] ?: id
        if (!event.matches(Regex("[a-f0-9]{64}"))) return
        val seen = prefs.getStringSet("seen_events", emptySet()) ?: emptySet()
        if (seen.contains(event)) return
        val route = data["route"] ?: return
        if (route != "/incidents/$id" && route != "/watchdog") return
        val severity = data["severity"] ?: return
        val state = data["state"] ?: return
        if (severity !in listOf("information", "warning", "degraded", "critical") || state !in listOf("attention", "resolved", "unreachable", "reachable")) return
        val manager = getSystemService(NotificationManager::class.java)
        if (!manager.areNotificationsEnabled()) return
        val urgent = data["urgent"] == "true" && severity == "critical" && route != "/watchdog"
        val channel = if (urgent) "prisme_urgent" else "prisme_normal"
        manager.createNotificationChannel(NotificationChannel(channel, if (urgent) "Prisme · Urgences" else "Prisme · Notifications", if (urgent) NotificationManager.IMPORTANCE_HIGH else NotificationManager.IMPORTANCE_DEFAULT))
        val intent = Intent(this, MainActivity::class.java).setAction(Intent.ACTION_VIEW)
            .setData(Uri.parse("prisme://incidents/$id")).putExtra("prisme_route", route)
            .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        val pending = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val fr = resources.configuration.locales[0].language == "fr"
        val text = when (state) {
            "resolved" -> if (fr) "Incident résolu · ouvrir Prisme" else "Incident resolved · open Prisme"
            "unreachable" -> if (fr) "Centre de contrôle injoignable · état des sites inconnu" else "Control plane unreachable · site health unknown"
            "reachable" -> if (fr) "Centre de contrôle joignable" else "Control plane reachable"
            else -> if (fr) "Incident à consulter dans Prisme" else "Incident needs attention in Prisme"
        }
        val notification = Notification.Builder(this, channel).setSmallIcon(R.drawable.ic_notification)
            .setContentTitle("Prisme").setContentText(text).setContentIntent(pending).setAutoCancel(true)
            .setVisibility(Notification.VISIBILITY_PRIVATE).setGroup("prisme_incidents")
            .setOnlyAlertOnce(false).setTimeoutAfter(300000).build()
        manager.notify(id, 0, notification)
        prefs.edit().putStringSet("seen_events", (seen.takeLastBounded(127) + event).toSet()).apply()
    }
}

private fun Set<String>.takeLastBounded(size: Int): List<String> = toList().takeLast(size)
