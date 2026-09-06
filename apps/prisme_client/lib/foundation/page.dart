import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'controller.dart';

class FoundationPage extends StatefulWidget {
  const FoundationPage({
    super.key,
    required this.controller,
    required this.preferences,
  });
  final FoundationController controller;
  final PrismePreferences preferences;
  @override
  State<FoundationPage> createState() => _FoundationPageState();
}

class _FoundationPageState extends State<FoundationPage> {
  final _secret = TextEditingController();
  final _device = TextEditingController(text: 'development-client');
  String t(String fr, String en) =>
      Localizations.localeOf(context).languageCode == 'fr' ? fr : en;
  @override
  void dispose() {
    _secret.dispose();
    _device.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) {
      final c = widget.controller;
      final page = c.page;
      return Scaffold(
        body: SafeArea(
          child: PrismePageFrame(
            header: PrismeSectionHeader(
              title: t('Prisme · Fondation', 'Prisme · Foundation'),
              subtitle: t(
                'Connexion et agent de test',
                'Connection and test agent',
              ),
              actions: [
                PrismeIconButton(
                  icon: LucideIcons.languages,
                  label: t('Passer en anglais', 'Switch to French'),
                  onPressed: () =>
                      widget.preferences.locale = Locale(t('en', 'fr')),
                ),
              ],
            ),
            children: [
              PrismeBanner(
                title: t(
                  'Environnement de développement',
                  'Development environment',
                ),
                message: t(
                  'Connectez-vous avec le secret local. La session dure une heure.',
                  'Sign in with the local secret. The session lasts one hour.',
                ),
              ),
              if (c.error != null)
                PrismeBanner(
                  tone: PrismeTone.attention,
                  title: t('Connexion non vérifiée', 'Connection not verified'),
                  message: _error(c.error!),
                ),
              if (c.identity == null)
                PrismeSurface(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      PrismeTextField(
                        label: t('Nom de cet appareil', 'Device name'),
                        controller: _device,
                        enabled: !c.busy,
                      ),
                      const SizedBox(height: 20),
                      PrismeSecretField(
                        label: t(
                          'Secret de développement',
                          'Development secret',
                        ),
                        controller: _secret,
                        enabled: !c.busy,
                      ),
                      const SizedBox(height: 20),
                      PrismeButton(
                        label: t('Se connecter', 'Sign in'),
                        loading: c.busy,
                        variant: PrismeButtonVariant.primary,
                        onPressed: c.busy
                            ? null
                            : () async {
                                final secret = _secret.text;
                                _secret.clear();
                                await c.login(secret, _device.text.trim());
                              },
                      ),
                    ],
                  ),
                )
              else ...[
                PrismeSurface(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      PrismeSectionHeader(
                        title: c.connected
                            ? t('Backend connecté', 'Backend connected')
                            : t(
                                'État du backend inconnu',
                                'Backend state unknown',
                              ),
                        subtitle: c.identity!.device_name,
                      ),
                      const SizedBox(height: 20),
                      Wrap(
                        spacing: 12,
                        runSpacing: 12,
                        children: [
                          PrismeButton(
                            label: t('Actualiser', 'Refresh'),
                            loading: c.busy,
                            onPressed: c.busy ? null : c.refresh,
                          ),
                          PrismeButton(
                            label: t('Se déconnecter', 'Sign out'),
                            onPressed: c.busy ? null : c.logout,
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
                PrismeSectionHeader(
                  title: t('Agents de test', 'Test agents'),
                  subtitle: page == null
                      ? null
                      : t(
                          'Dernière lecture : ${page.observed_at}',
                          'Last read: ${page.observed_at}',
                        ),
                ),
                if (page != null && page.items.isEmpty)
                  PrismeSurface(
                    child: Text(
                      t(
                        'Aucun agent enrôlé. Lancez le test d’enrôlement local, puis actualisez.',
                        'No agents enrolled. Run the local enrollment test, then refresh.',
                      ),
                    ),
                  ),
                if (page != null)
                  for (final agent in page.items)
                    PrismeSurface(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          PrismeSectionHeader(title: agent.name),
                          const SizedBox(height: 16),
                          PrismeKeyValue(
                            label: t('Identité', 'Identity'),
                            value: agent.id,
                          ),
                          const SizedBox(height: 12),
                          PrismeKeyValue(
                            label: t('Dernier signal', 'Last heartbeat'),
                            value:
                                agent.last_seen_at ??
                                t('Pas encore reçu', 'Not received yet'),
                          ),
                          const SizedBox(height: 12),
                          Text(
                            agent.revoked
                                ? t('Identité révoquée', 'Identity revoked')
                                : t(
                                    'Capacité : signal de présence uniquement',
                                    'Capability: self heartbeat only',
                                  ),
                          ),
                        ],
                      ),
                    ),
                if (page != null && page.next_cursor.isNotEmpty)
                  PrismeButton(
                    label: t('Page suivante', 'Next page'),
                    onPressed: c.busy ? null : () => c.refresh(nextPage: true),
                  ),
              ],
            ],
          ),
        ),
      );
    },
  );
  String _error(String code) => switch (code) {
    'unauthorized' || 'session_expired' => t(
      'Secret refusé ou session expirée. Reconnectez-vous.',
      'Secret rejected or session expired. Sign in again.',
    ),
    'timeout' || 'offline' => t(
      'Le backend est injoignable. Vérifiez la connexion et réessayez.',
      'The backend is unavailable. Check the connection and retry.',
    ),
    'untrusted_server' => t(
      'Le certificat du serveur n’est pas reconnu. Vérifiez la configuration locale.',
      'The server certificate is not trusted. Check the local configuration.',
    ),
    _ => t(
      'La réponse n’a pas pu être validée. Réessayez après vérification du backend.',
      'The response could not be verified. Check the backend and retry.',
    ),
  };
}
