import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:xterm/xterm.dart';
import '../api/connection.dart';
import '../api/generated.dart';
import 'controller.dart';

TerminalTheme terminalTheme(BuildContext context) {
  final c = context.prismeColors;
  return TerminalTheme(
    cursor: c.accent,
    selection: c.accentSubtle,
    foreground: c.text,
    background: c.surface,
    black: c.surface,
    white: c.text,
    red: c.critical,
    green: c.healthy,
    yellow: c.attention,
    blue: c.accent,
    magenta: c.attention,
    cyan: c.accent,
    brightBlack: c.muted,
    brightRed: c.critical,
    brightGreen: c.healthy,
    brightYellow: c.attention,
    brightBlue: c.accent,
    brightMagenta: c.attention,
    brightCyan: c.accent,
    brightWhite: c.text,
    searchHitBackground: c.attentionSubtle,
    searchHitBackgroundCurrent: c.accentSubtle,
    searchHitForeground: c.text,
  );
}

class OperationsPage extends StatefulWidget {
  const OperationsPage({
    super.key,
    required this.send,
    required this.prove,
    this.openSocket,
    this.recovery = false,
    this.incident = '',
    this.platformActivity,
  });
  final ApiTransport send;
  final OperationProof prove;
  final Future<WebSocket> Function(String, String)? openSocket;
  final bool recovery;
  final String incident;
  final void Function(bool)? platformActivity;
  @override
  State<OperationsPage> createState() => _OperationsPageState();
}

class _OperationsPageState extends State<OperationsPage>
    with WidgetsBindingObserver {
  late final OperationsController c;
  String? target, active, error, revision, uploadId;
  String section = 'terminal', original = '', secretId = '';
  bool busy = false, editable = false, split = false;
  int secretRevision = 0, progress = 0;
  double fontSize = 14;
  List<Map<String, dynamic>> entries = [];
  final path = TextEditingController(),
      editor = TextEditingController(),
      search = TextEditingController();
  final secretName = TextEditingController(),
      secretValue = TextEditingController(),
      scope = TextEditingController(text: 'default');
  final envPath = TextEditingController(), envName = TextEditingController();
  Uint8List? uploadBytes;
  String? downloadId;
  final downloadBytes = <int>[];
  Timer? hideTimer;
  String? revealed;
  Map<String, dynamic>? diagnostics;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  List<Map<String, dynamic>> get targets =>
      (c.overview?['targets'] as List? ?? [])
          .map((v) => Map<String, dynamic>.from(v as Map))
          .toList();
  Map<String, dynamic>? get selected =>
      targets.where((v) => v['id'] == target).firstOrNull;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    c = OperationsController(widget.send, widget.prove, widget.openSocket);
    WidgetsBinding.instance.addPostFrameCallback((_) => run(refresh));
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    hideTimer?.cancel();
    c.dispose();
    for (final f in [
      path,
      editor,
      search,
      secretName,
      secretValue,
      scope,
      envPath,
      envName,
    ]) {
      f.dispose();
    }
    uploadBytes?.fillRange(0, uploadBytes!.length, 0);
    downloadBytes.fillRange(0, downloadBytes.length, 0);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) {
      hideTimer?.cancel();
      revealed = null;
      secretValue.clear();
      if (mounted) setState(() {});
    }
  }

  Future<void> run(Future<void> Function() fn) async {
    if (busy || !mounted) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await fn();
    } on ApiFailure catch (e) {
      if (mounted) setState(() => error = e.code);
    } catch (_) {
      if (mounted) setState(() => error = 'operation_unavailable');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> refresh() async {
    await c.refresh();
    if (!mounted) return;
    target ??= targets.firstOrNull?['id'] as String?;
    if (path.text.isEmpty) {
      path.text = (selected?['roots'] as List?)?.firstOrNull as String? ?? '';
    }
  }

  Future<bool> confirm(
    String title,
    String impact, {
    String? exactTarget,
  }) => showPrismeConfirmation(
    context: context,
    plan: PrismeActionPlan(
      title: title,
      actionLabel: title,
      target:
          exactTarget ??
          '${selected?['name'] ?? target} · ${selected?['user'] ?? ''}',
      environment: selected?['environment'] == 'production'
          ? PrismeEnvironment.production
          : PrismeEnvironment.development,
      impact: impact,
      verification: t(
        'Vérification côté serveur et audit de l’opération.',
        'Server verification and operation audit.',
      ),
      rollback: t(
        'Le terminal conserve les pouvoirs du compte SSH. Les fichiers remplacés possèdent une copie de sécurité.',
        'The terminal has the SSH account permissions. Replaced files have a backup.',
      ),
    ),
  );
  Future<void> open({String? id, String? sessionTarget}) async {
    final targetId = sessionTarget ?? target;
    if (targetId == null) return;
    if (!await confirm(
      t('Ouvrir le terminal', 'Open terminal'),
      t(
        'Accès interactif au compte sélectionné. Les opérations automatiques sur cette cible seront bloquées.',
        'Interactive access as the selected account. Automatic operations on this target will be blocked.',
      ),
      exactTarget: targetId,
    )) {
      return;
    }
    final terminal = await c.terminal(
      targetId,
      incident: widget.incident,
      id: id,
    );
    if (!mounted) return;
    terminal.terminal.onOutput = (v) async {
      if (v.length > 1 && (v.contains('\n') || v.contains('\r'))) {
        if (!await confirm(
          t('Coller plusieurs lignes', 'Paste multiple lines'),
          t(
            'Ces lignes peuvent exécuter plusieurs commandes.',
            'These lines can execute multiple commands.',
          ),
          exactTarget: terminal.target,
        )) {
          return;
        }
      }
      terminal.input(v);
    };
    setState(() => active = terminal.id);
    await refresh();
  }

  Future<void> browse() async {
    if (target == null) return;
    final result = await c.execute('file_list', target!, {
      'path': path.text,
    }, proof: false);
    if (!mounted) return;
    setState(() {
      entries = (result['entries'] as List)
          .map((v) => Map<String, dynamic>.from(v as Map))
          .toList();
      revision = null;
      editor.clear();
      original = '';
    });
  }

  Future<void> readFile(String file) async {
    final result = await c.execute('file_read', target!, {'path': file});
    if (!mounted) return;
    setState(() {
      path.text = file;
      editor.text = result['content'] as String;
      original = editor.text;
      revision = result['revision'] as String;
      editable = result['editable'] == true;
      entries = [];
    });
  }

  Future<void> save() async {
    final content = editor.text, file = path.text, rev = revision;
    if (rev == null) return;
    if (!await confirm(
      t('Enregistrer le fichier', 'Save file'),
      t(
        'Remplacement atomique après vérification de la version et création d’une copie privée.',
        'Atomic replacement after checking the version and creating a private backup.',
      ),
      exactTarget: '$target · $file',
    )) {
      return;
    }
    final result = await c.execute('file_save', target!, {
      'path': file,
      'revision': rev,
      'content': content,
    });
    if (!mounted) return;
    setState(() {
      revision = result['revision'] as String;
      original = content;
    });
  }

  Future<void> upload() async {
    widget.platformActivity?.call(true);
    FilePickerResult? picked;
    try {
      picked = await FilePicker.platform.pickFiles(
        withData: true,
        allowMultiple: false,
      );
    } finally {
      widget.platformActivity?.call(false);
    }
    if (picked == null || !mounted) return;
    final file = picked.files.single;
    if (file.size > 1024 * 1024 || file.bytes == null) {
      throw const ApiFailure('file_too_large');
    }
    final destination = '${path.text}/${file.name}';
    if (!await confirm(
      t('Téléverser le fichier', 'Upload file'),
      t(
        'Taille maximale : 1 Mio. La version distante sera vérifiée au remplacement.',
        'Maximum size: 1 MiB. The remote version will be checked before replacement.',
      ),
      exactTarget: '$target · $destination',
    )) {
      return;
    }
    final stat = await c.execute('file_stat', target!, {
      'path': destination,
    }, proof: false);
    final bytes = file.bytes!;
    final result = await c.execute('upload_begin', target!, {
      'path': destination,
      'revision': stat['revision'],
      'size': bytes.length,
      'digest': sha256.convert(bytes).toString(),
    });
    uploadId = result['id'] as String;
    uploadBytes = bytes;
    await resumeUpload();
  }

  Future<void> resumeUpload() async {
    final bytes = uploadBytes, id = uploadId;
    if (bytes == null || id == null) return;
    final state = await c.execute('upload_status', target!, {
      'id': id,
    }, proof: false);
    var offset = state['offset'] as int;
    while (offset < bytes.length) {
      final result = await c.execute('upload_chunk', target!, {
        'id': id,
        'offset': offset,
        'base64': base64Encode(
          bytes.sublist(offset, (offset + 48 * 1024).clamp(0, bytes.length)),
        ),
      }, proof: false);
      offset = result['offset'] as int;
      if (!mounted) return;
      setState(() => progress = offset);
    }
    await c.execute('upload_commit', target!, {'id': id});
    bytes.fillRange(0, bytes.length, 0);
    uploadBytes = null;
    uploadId = null;
    await browse();
  }

  Future<void> download() async {
    if (downloadId == null) {
      final start = await c.execute('download_begin', target!, {
        'path': path.text,
      });
      downloadId = start['id'] as String;
    }
    final bytes = downloadBytes;
    var offset = bytes.length;
    while (true) {
      final result = await c.execute('download_chunk', target!, {
        'id': downloadId,
        'offset': offset,
      }, proof: false);
      bytes.addAll(base64Decode(result['base64'] as String));
      offset = result['offset'] as int;
      if (bytes.length > 1024 * 1024) throw const ApiFailure('file_too_large');
      if (offset >= (result['size'] as int)) break;
    }
    widget.platformActivity?.call(true);
    try {
      final output = await FilePicker.platform.saveFile(
        fileName: path.text.split('/').last,
        bytes: Uint8List.fromList(bytes),
      );
      if (output != null && Platform.isWindows) {
        await File(output).writeAsBytes(bytes, flush: true);
      }
    } finally {
      widget.platformActivity?.call(false);
      bytes.fillRange(0, bytes.length, 0);
      bytes.clear();
      final id = downloadId;
      downloadId = null;
      if (id != null) {
        await c.execute('upload_cancel', target!, {'id': id}, proof: false);
      }
    }
  }

  Future<void> putSecret() async {
    if (!await confirm(
      t('Enregistrer le secret', 'Save secret'),
      t(
        'La nouvelle valeur sera chiffrée et limitée à cette destination.',
        'The new value will be encrypted and restricted to this destination.',
      ),
    )) {
      return;
    }
    final value = secretValue.text;
    secretValue.clear();
    final response = await c.execute('secret_put', target!, {
      'secret': {
        'id': secretId,
        'name': secretName.text,
        'scope': scope.text,
        'kind': 'secret',
        'destination': target,
        'revision': secretRevision,
        'expires_at': DateTime.now()
            .toUtc()
            .add(const Duration(days: 90))
            .toIso8601String(),
      },
      'value': value,
    });
    final secret = response['secret'] as Map;
    secretId = secret['id'] as String;
    secretRevision = secret['revision'] as int;
    await refresh();
  }

  Future<void> reveal(Map<String, dynamic> secret) async {
    final result = await c.execute(
      'secret_reveal',
      secret['destination'] as String,
      {'id': secret['id'], 'revision': secret['revision']},
    );
    if (!mounted) return;
    hideTimer?.cancel();
    setState(() => revealed = result['value'] as String);
    hideTimer = Timer(const Duration(seconds: 30), () {
      if (mounted) setState(() => revealed = null);
    });
  }

  Future<void> applyEnv() async {
    if (secretId.isEmpty) return;
    final file = envPath.text;
    if (!await confirm(
      t('Écrire le fichier .env', 'Write .env file'),
      t(
        'Le secret sera résolu sur le serveur et écrit avec des permissions privées.',
        'The secret will be resolved on the server and written with private permissions.',
      ),
      exactTarget: '$target · $file',
    )) {
      return;
    }
    final stat = await c.execute('file_stat', target!, {
      'path': file,
    }, proof: false);
    await c.execute('env_apply', target!, {
      'path': file,
      'revision': stat['revision'],
      'variables': <String, String>{},
      'secrets': {
        envName.text: {'id': secretId, 'revision': secretRevision},
      },
    });
  }

  String get errorText => switch (error) {
    'operations_disabled' => t(
      'Les accès Terminal, fichiers et coffre ne sont pas configurés sur ce serveur.',
      'Terminal, files and vault access are not configured on this server.',
    ),
    'revision_conflict' => t(
      'La version ou l’état a changé. Rechargez avant de préparer une nouvelle action.',
      'The version or state changed. Reload before preparing another action.',
    ),
    'host_reconciliation_required' => t(
      'La cible est verrouillée ou nécessite une réconciliation sur l’hôte. Vérifiez les processus en cours.',
      'The target is locked or requires reconciliation on the host. Check running processes.',
    ),
    'operation_denied' || 'passkey_session_required' => t(
      'Accès refusé. Utilisez une session passkey et vérifiez les capacités, chemins et clés biométriques.',
      'Access denied. Use a passkey session and check capabilities, paths and biometric keys.',
    ),
    _ => t(
      'Opération interrompue ou indisponible. Vérifiez la connexion et l’état distant avant de réessayer.',
      'Operation interrupted or unavailable. Check the connection and remote state before retrying.',
    ),
  };
  @override
  Widget build(BuildContext context) => PrismePageFrame(
    header: PrismeSectionHeader(
      title: t('Terminal, fichiers et secrets', 'Terminal, files and secrets'),
      subtitle: widget.incident.isEmpty ? null : 'Incident ${widget.incident}',
      actions: [
        PrismeButton(
          label: t('Actualiser', 'Refresh'),
          loading: busy,
          onPressed: busy ? null : () => run(refresh),
        ),
      ],
    ),
    children: [
      if (widget.recovery)
        PrismeBanner(
          tone: PrismeTone.critical,
          title: t(
            'Mode secours · identité indépendante',
            'Recovery mode · independent identity',
          ),
          message: t(
            'Les opérations sont effectuées sur le VPS. La reprise des automatismes exige une réconciliation sur l’hôte.',
            'Operations run on the VPS. Resuming automation requires reconciliation on the host.',
          ),
        ),
      if (error != null)
        PrismeBanner(
          tone: PrismeTone.attention,
          title: t('Action à vérifier', 'Action needs checking'),
          message: errorText,
        ),
      if (targets.isNotEmpty)
        PrismeSelect<String>(
          label: t('Serveur et identité SSH', 'Server and SSH identity'),
          options: [
            for (final item in targets)
              PrismeOption(
                item['id'] as String,
                '${item['name']} · ${item['user']} · ${item['environment']}',
              ),
          ],
          value: target,
          onChanged: busy || uploadId != null || downloadId != null
              ? null
              : (v) {
                  setState(() {
                    target = v;
                    path.text =
                        (selected?['roots'] as List?)?.firstOrNull as String? ??
                        '';
                    revision = null;
                    entries = [];
                    editor.clear();
                    original = '';
                    revealed = null;
                    secretValue.clear();
                  });
                },
        ),
      if (targets.isEmpty && c.overview != null)
        PrismeSurface(
          child: Text(
            t(
              'Aucune cible d’administration configurée.',
              'No administration target configured.',
            ),
          ),
        ),
      if (target != null) ...[
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final item in {
              'terminal': 'Terminal',
              'files': t('Fichiers', 'Files'),
              if (c.overview?['vault'] == true)
                'secrets': t('Secrets', 'Secrets'),
            }.entries)
              ChoiceChip(
                label: Text(item.value),
                selected: section == item.key,
                onSelected: busy
                    ? null
                    : (_) => setState(() {
                        section = item.key;
                        revealed = null;
                        secretValue.clear();
                      }),
              ),
          ],
        ),
        if (section == 'terminal') ...terminalWidgets(),
        if (section == 'files') ...fileWidgets(),
        if (section == 'secrets') ...secretWidgets(),
      ],
    ],
  );
  List<Widget> terminalWidgets() {
    final remote = (c.overview?['sessions'] as List? ?? []).cast<Map>();
    final local = c.terminals[active];
    return [
      if (selected?['diagnostics'] == true) ...[
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            PrismeButton(
              label: t(
                'Diagnostic système et conteneurs',
                'System and container diagnostics',
              ),
              onPressed: busy
                  ? null
                  : () => run(() async {
                      final result = await c.execute(
                        'diagnostics',
                        target!,
                        {},
                        proof: false,
                      );
                      if (mounted) setState(() => diagnostics = result);
                    }),
            ),
            for (final container in (selected?['containers'] as List? ?? []))
              PrismeButton(
                label: 'Logs · $container',
                onPressed: busy
                    ? null
                    : () => run(() async {
                        final result = await c.execute('diagnostics', target!, {
                          'container': container,
                        }, proof: false);
                        if (mounted) setState(() => diagnostics = result);
                      }),
              ),
          ],
        ),
        if (diagnostics != null) ...[
          Text(
            '${diagnostics!['target']} · ${diagnostics!['source']} · ${diagnostics!['observed_at']} · ${diagnostics!['state']}',
          ),
          PrismeCodeBlock(text: diagnostics!['text'] as String),
        ],
      ],
      PrismeBanner(
        title: t('Compte distant', 'Remote account'),
        message: t(
          'Le terminal dispose des droits du compte SSH choisi. Son contenu reste en mémoire ; aucune transcription n’est enregistrée.',
          'The terminal has the selected SSH account permissions. Its contents stay in memory; no transcript is recorded.',
        ),
      ),
      Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          PrismeButton(
            label: t('Nouvelle session', 'New session'),
            onPressed: busy || selected?['terminal'] != true
                ? null
                : () => run(() => open()),
          ),
          for (final session in remote)
            PrismeButton(
              label:
                  '${session['target']} · ${(session['id'] as String).substring(0, 8)}',
              onPressed: busy
                  ? null
                  : () {
                      final id = session['id'] as String;
                      if (c.terminals.containsKey(id)) {
                        setState(() => active = id);
                      } else {
                        run(
                          () => open(
                            id: id,
                            sessionTarget: session['target'] as String,
                          ),
                        );
                      }
                    },
            ),
          if (MediaQuery.sizeOf(context).width > 1000 && c.terminals.length > 1)
            FilterChip(
              label: Text(t('Diviser', 'Split')),
              selected: split,
              onSelected: (v) => setState(() => split = v),
            ),
        ],
      ),
      if (local != null)
        if (split && MediaQuery.sizeOf(context).width > 1000)
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              for (final session in c.terminals.values.take(2))
                Expanded(child: terminalPane(session)),
            ],
          )
        else
          terminalPane(local),
    ];
  }

  Widget terminalPane(TerminalSession session) => AnimatedBuilder(
    animation: session,
    builder: (context, _) => PrismeSurface(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            '${session.target} · ${session.user} · ${session.environment} · ${session.state}',
          ),
          if (session.environment == 'production')
            PrismeEnvironmentBadge(environment: PrismeEnvironment.production),
          if (!session.connected)
            Text(
              t(
                'Liaison interrompue. Aucune saisie ne sera rejouée. La commande peut encore être active.',
                'Connection interrupted. Input will not be replayed. The command may still be active.',
              ),
            ),
          if (session.gap)
            Text(
              t(
                'Une partie de la sortie n’est plus disponible.',
                'Some output is no longer available.',
              ),
            ),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              PrismeButton(
                label: t('Copier', 'Copy'),
                onPressed: () {
                  final selection = session.selection.selection;
                  if (selection != null) {
                    Clipboard.setData(
                      ClipboardData(
                        text: session.terminal.buffer.getText(selection),
                      ),
                    );
                  }
                },
              ),
              PrismeButton(
                label: t('Coller', 'Paste'),
                onPressed: session.connected
                    ? () => run(() => paste(session))
                    : null,
              ),
              PrismeButton(
                label: t('Reconnecter', 'Reconnect'),
                onPressed: busy || session.connected
                    ? null
                    : () => run(
                        () =>
                            open(id: session.id, sessionTarget: session.target),
                      ),
              ),
              PrismeButton(
                label: t('Fermer la session', 'Close session'),
                onPressed: busy
                    ? null
                    : () => run(() async {
                        if (await confirm(
                          t('Fermer la session', 'Close session'),
                          t(
                            'Demander l’arrêt du processus interactif. Vérifiez son arrêt effectif avant de réconcilier.',
                            'Request termination of the interactive process. Verify it stopped before reconciling.',
                          ),
                          exactTarget: session.target,
                        )) {
                          await c.execute('terminal_close', session.target, {
                            'id': session.id,
                          }, proof: false);
                        }
                      }),
              ),
              IconButton(
                tooltip: t('Réduire la police', 'Smaller font'),
                onPressed: () =>
                    setState(() => fontSize = (fontSize - 1).clamp(10, 24)),
                icon: const Icon(Icons.text_decrease),
              ),
              IconButton(
                tooltip: t('Agrandir la police', 'Larger font'),
                onPressed: () =>
                    setState(() => fontSize = (fontSize + 1).clamp(10, 24)),
                icon: const Icon(Icons.text_increase),
              ),
            ],
          ),
          SizedBox(
            height: MediaQuery.sizeOf(context).width < 600 ? 320 : 430,
            child: TerminalView(
              session.terminal,
              theme: terminalTheme(context),
              controller: session.selection,
              readOnly: !session.connected,
              textStyle: TerminalStyle(
                fontSize: fontSize,
                fontFamily: 'IBM Plex Mono',
              ),
              shortcuts: const {},
              onKeyEvent: (_, event) {
                if (event is KeyDownEvent &&
                    HardwareKeyboard.instance.isControlPressed &&
                    event.logicalKey == LogicalKeyboardKey.keyV) {
                  run(() => paste(session));
                  return KeyEventResult.handled;
                }
                return KeyEventResult.ignored;
              },
            ),
          ),
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: [
                for (final entry in {
                  'Esc': '\x1b',
                  'Ctrl+C': '\x03',
                  'Tab': '\t',
                  '↑': '\x1b[A',
                  '↓': '\x1b[B',
                  '←': '\x1b[D',
                  '→': '\x1b[C',
                  '/': '/',
                }.entries)
                  TextButton(
                    onPressed: session.connected
                        ? () => session.input(entry.value)
                        : null,
                    child: Text(entry.key),
                  ),
              ],
            ),
          ),
          PrismeTextField(
            label: t('Rechercher dans le tampon', 'Search scrollback'),
            controller: search,
            onChanged: (_) => setState(() {}),
          ),
          if (search.text.isNotEmpty)
            PrismeCodeBlock(
              text: session.terminal.buffer
                  .getText()
                  .split('\n')
                  .where((line) => line.contains(search.text))
                  .take(50)
                  .join('\n'),
            ),
        ],
      ),
    ),
  );
  Future<void> paste(TerminalSession session) async {
    final clipboard = await Clipboard.getData(Clipboard.kTextPlain);
    final value = clipboard?.text;
    if (value == null || !mounted) return;
    if ((value.contains('\n') || value.contains('\r')) &&
        !await confirm(
          t('Coller plusieurs lignes', 'Paste multiple lines'),
          t(
            'Le collage peut exécuter plusieurs commandes.',
            'Pasting may execute multiple commands.',
          ),
          exactTarget: session.target,
        )) {
      return;
    }
    session.terminal.paste(value);
  }

  List<Widget> fileWidgets() => [
    PrismeTextField(
      label: t('Chemin distant autorisé', 'Allowed remote path'),
      controller: path,
      enabled: !busy && uploadId == null && downloadId == null,
      onChanged: (_) => setState(() {
        revision = null;
        editor.clear();
        original = '';
        entries = [];
      }),
    ),
    Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        PrismeButton(
          label: t('Parcourir', 'Browse'),
          onPressed: busy || selected?['files'] != true
              ? null
              : () => run(browse),
        ),
        PrismeButton(
          label: t('Téléverser ici', 'Upload here'),
          onPressed: busy || revision != null ? null : () => run(upload),
        ),
        if (revision != null)
          PrismeButton(
            label: t('Télécharger', 'Download'),
            onPressed: busy ? null : () => run(download),
          ),
        if (uploadId != null)
          PrismeButton(
            label: t('Reprendre le transfert', 'Resume transfer'),
            onPressed: busy ? null : () => run(resumeUpload),
          ),
      ],
    ),
    if (uploadBytes != null)
      LinearProgressIndicator(value: progress / uploadBytes!.length),
    for (final entry in entries)
      ListTile(
        title: Text(entry['name'] as String),
        subtitle: Text(
          '${entry['mode']} · ${entry['uid']}:${entry['gid']} · ${entry['size']} B',
        ),
        leading: Icon(
          entry['directory'] == true ? LucideIcons.folder : LucideIcons.file,
        ),
        onTap: busy
            ? null
            : () => run(() async {
                final file = '${path.text}/${entry['name']}';
                if (entry['directory'] == true) {
                  path.text = file;
                  await browse();
                } else {
                  await readFile(file);
                }
              }),
      ),
    if (revision != null) ...[
      if (!editable)
        Text(
          t(
            'Aperçu masqué : édition désactivée pour préserver les valeurs sensibles.',
            'Redacted preview: editing disabled to preserve sensitive values.',
          ),
        ),
      TextField(
        controller: editor,
        readOnly: !editable || busy,
        maxLines: 12,
        autocorrect: false,
        enableSuggestions: false,
        enableIMEPersonalizedLearning: false,
        style: PrismeTheme.mono(context),
        onChanged: (_) => setState(() {}),
      ),
      if (editor.text != original)
        PrismeDiffView(
          lines: [
            for (final line in redactDiff(original).split('\n'))
              PrismeDiffLine(redactDiff(line), PrismeDiffKind.removed),
            for (final line in redactDiff(editor.text).split('\n'))
              PrismeDiffLine(redactDiff(line), PrismeDiffKind.added),
          ],
        ),
      PrismeButton(
        label: t('Enregistrer avec copie de sécurité', 'Save with backup'),
        onPressed: busy || !editable || editor.text == original
            ? null
            : () => run(save),
      ),
    ],
  ];
  List<Widget> secretWidgets() => [
    Text(
      t(
        'Valeurs masquées. Révélation limitée à 30 secondes. Expiration des nouvelles valeurs : 90 jours.',
        'Values hidden. Reveal limited to 30 seconds. New values expire after 90 days.',
      ),
    ),
    for (final item
        in (c.overview?['secrets'] as List? ?? []).cast<Map>().where(
          (s) => s['destination'] == target,
        ))
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('${item['name']} · ${item['scope']} · v${item['revision']}'),
            Text('${item['expires_at']}'),
            Wrap(
              spacing: 8,
              children: [
                PrismeButton(
                  label: t('Révéler', 'Reveal'),
                  onPressed: busy
                      ? null
                      : () =>
                            run(() => reveal(Map<String, dynamic>.from(item))),
                ),
                PrismeButton(
                  label: t('Sélectionner / rotation', 'Select / rotate'),
                  onPressed: busy
                      ? null
                      : () => setState(() {
                          secretId = item['id'] as String;
                          secretRevision = item['revision'] as int;
                          secretName.text = item['name'] as String;
                          scope.text = item['scope'] as String;
                          secretValue.clear();
                        }),
                ),
              ],
            ),
          ],
        ),
      ),
    if (revealed != null)
      Focus(
        onFocusChange: (focused) {
          if (!focused && mounted) setState(() => revealed = null);
        },
        child: PrismeSurface(
          child: Column(
            children: [
              Text(revealed!),
              PrismeButton(
                label: t('Masquer', 'Hide'),
                onPressed: () => setState(() => revealed = null),
              ),
            ],
          ),
        ),
      ),
    PrismeTextField(
      label: t('Nom du secret', 'Secret name'),
      controller: secretName,
      enabled: !busy,
    ),
    PrismeTextField(
      label: t('Projet / environnement', 'Project / environment'),
      controller: scope,
      enabled: !busy,
    ),
    PrismeSecretField(
      label: t('Nouvelle valeur', 'New value'),
      controller: secretValue,
      enabled: !busy,
    ),
    Wrap(
      spacing: 8,
      children: [
        PrismeButton(
          label: t('Enregistrer le secret', 'Save secret'),
          onPressed: busy ? null : () => run(putSecret),
        ),
        PrismeButton(
          label: t('Nouveau secret', 'New secret'),
          onPressed: busy
              ? null
              : () => setState(() {
                  secretId = '';
                  secretRevision = 0;
                  secretName.clear();
                  secretValue.clear();
                }),
        ),
      ],
    ),
    if (secretId.isNotEmpty) ...[
      PrismeTextField(
        label: t('Fichier .env de destination', 'Destination .env file'),
        controller: envPath,
      ),
      PrismeTextField(
        label: t('Nom de la variable', 'Variable name'),
        controller: envName,
      ),
      PrismeButton(
        label: t(
          'Écrire un .env avec ce secret',
          'Write .env with this secret',
        ),
        onPressed: busy ? null : () => run(applyEnv),
      ),
    ],
  ];
}
