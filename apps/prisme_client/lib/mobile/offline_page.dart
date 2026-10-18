import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';

class OfflinePage extends StatelessWidget {
  const OfflinePage({super.key, required this.snapshot, required this.close});
  final Map<String, dynamic> snapshot;
  final VoidCallback close;
  @override
  Widget build(BuildContext context) {
    String t(String fr, String en) => context.prismeStrings.choose(fr, en);
    return Scaffold(
      body: SafeArea(
        child: PrismePageFrame(
          header: PrismeSectionHeader(
            title: t('Dernier état enregistré', 'Last saved state'),
            actions: [
              PrismeButton(
                label: t('Revenir à la connexion', 'Return to sign in'),
                onPressed: close,
              ),
            ],
          ),
          children: [
            PrismeBanner(
              title: t('Cache · lecture seule', 'Cache · read only'),
              message: t(
                'Ces données sont anciennes. Aucune action n’est disponible ni mise en attente. L’accès de secours indépendant reste à configurer.',
                'These are past observations. Actions are unavailable and are never queued. Independent recovery access still needs configuration.',
              ),
            ),
            for (final domain in [
              '/v1/servers',
              '/v1/incidents',
              '/v1/deployments',
            ])
              if (snapshot[domain] is Map)
                PrismeSurface(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(switch (domain) {
                        '/v1/servers' => t(
                          'Machines · état ancien',
                          'Machines · stale state',
                        ),
                        '/v1/incidents' => t(
                          'Incidents connus · état ancien',
                          'Known incidents · stale state',
                        ),
                        _ => t(
                          'Déploiements · état ancien',
                          'Deployments · stale state',
                        ),
                      }, style: Theme.of(context).textTheme.titleLarge),
                      Text(
                        '${t('Enregistré le', 'Saved at')} ${snapshot[domain]['captured_at']}',
                      ),
                      for (final item in snapshot[domain]['items'] as List)
                        Padding(
                          padding: const EdgeInsets.symmetric(vertical: 8),
                          child: Text((item as Map).values.join(' · ')),
                        ),
                    ],
                  ),
                ),
          ],
        ),
      ),
    );
  }
}
