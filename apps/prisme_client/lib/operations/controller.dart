import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:xterm/xterm.dart';
import '../api/generated.dart';
import '../api/connection.dart';

typedef OperationProof = Future<String> Function(Map<String, dynamic> intent);

class OperationsController extends ChangeNotifier {
  OperationsController(this.send, this.prove, this.openSocket);
  final ApiTransport send;
  final OperationProof prove;
  final Future<WebSocket> Function(String, String)? openSocket;
  Map<String, dynamic>? overview;
  final terminals = <String, TerminalSession>{};
  bool disposed = false;
  Future<void> refresh() async {
    final result = (await PrismeApi(send).operationsOverview()).toJson();
    if (disposed) return;
    overview = result;
    notifyListeners();
  }

  Future<Map<String, dynamic>> execute(
    String action,
    String target,
    Map<String, dynamic> payload, {
    bool proof = true,
  }) async {
    final intent = <String, dynamic>{
      'action': action,
      'target': target,
      'payload': payload,
    };
    final ticket = proof ? await prove(intent) : '';
    if (disposed) throw const ApiFailure('session_expired');
    return (await PrismeApi(send).operationsExecute(
      OperationsExecuteRequest(
        intent: OperationIntent.fromJson(intent),
        proof: ticket,
      ),
    )).data;
  }

  Future<TerminalSession> terminal(
    String target, {
    String incident = '',
    String? id,
  }) async {
    final result = await execute(
      id == null ? 'terminal_open' : 'terminal_attach',
      target,
      id == null
          ? {'columns': 80, 'rows': 24, 'incident': incident}
          : {'id': id},
    );
    if (openSocket == null) throw const ApiFailure('terminal_unavailable');
    final sessionId = result['id'] as String;
    final session = terminals.putIfAbsent(
      sessionId,
      () => TerminalSession(sessionId, target),
    );
    // The server is authoritative after uncertain delivery. Never resend input.
    session.inputSequence = result['input_seq'] as int;
    session.user = result['user'] as String? ?? '';
    session.environment = result['environment'] as String? ?? '';
    await session.connect(
      await openSocket!(sessionId, result['ticket'] as String),
    );
    if (disposed) {
      session.dispose();
      throw const ApiFailure('session_expired');
    }
    notifyListeners();
    return session;
  }

  @override
  void dispose() {
    disposed = true;
    for (final t in terminals.values) {
      t.dispose();
    }
    terminals.clear();
    super.dispose();
  }
}

class TerminalSession extends ChangeNotifier {
  TerminalSession(this.id, this.target) {
    terminal.onOutput = input;
    terminal.onResize = (columns, rows, _, _) {
      if (connected) {
        socket?.add(
          jsonEncode({
            'type': 'resize',
            'columns': columns.clamp(20, 500),
            'rows': rows.clamp(5, 300),
          }),
        );
      }
    };
  }
  final String id, target;
  final Terminal terminal = Terminal(maxLines: 2000);
  final TerminalController selection = TerminalController();
  WebSocket? socket;
  StreamSubscription<dynamic>? subscription;
  bool connected = false, gap = false, disposed = false;
  String state = 'disconnected';
  String user = '', environment = '';
  int inputSequence = 0, outputSequence = 0;
  late ByteConversionSink _decoder;
  Future<void> connect(WebSocket ws) async {
    await subscription?.cancel();
    await socket?.close();
    socket = ws;
    connected = true;
    state = 'connected';
    _decoder = const Utf8Decoder(allowMalformed: true).startChunkedConversion(
      StringConversionSink.fromStringSink(_TerminalSink(terminal)),
    );
    subscription = ws.listen(
      (raw) {
        if (disposed) return;
        try {
          final message = jsonDecode(raw as String) as Map;
          gap = gap || message['gap'] == true;
          for (final frame in message['frames'] as List) {
            final seq = frame['seq'] as int;
            if (seq <= outputSequence) continue;
            if (outputSequence != 0 && seq != outputSequence + 1) gap = true;
            outputSequence = seq;
            _decoder.add(base64Decode(frame['data'] as String));
          }
          state = message['state'] as String;
          if (state != 'connected') connected = false;
          notifyListeners();
        } catch (_) {
          disconnect();
        }
      },
      onDone: disconnect,
      onError: (_) => disconnect(),
    );
    notifyListeners();
  }

  void input(String value) {
    if (!connected || disposed) return;
    // WebSocket add is an attempt, never proof of remote execution.
    final bytes = utf8.encode(value);
    for (var offset = 0; offset < bytes.length; offset += 4096) {
      socket?.add(
        jsonEncode({
          'type': 'input',
          'seq': ++inputSequence,
          'data': base64Encode(
            bytes.sublist(offset, (offset + 4096).clamp(0, bytes.length)),
          ),
        }),
      );
    }
  }

  void disconnect() {
    connected = false;
    if (state == 'connected') state = 'disconnected';
    if (!disposed) notifyListeners();
  }

  @override
  void dispose() {
    if (disposed) return;
    disposed = true;
    subscription?.cancel();
    socket?.close();
    terminal.onOutput = null;
    terminal.onResize = null;
    selection.dispose();
    super.dispose();
  }
}

class _TerminalSink implements StringSink {
  _TerminalSink(this.terminal);
  final Terminal terminal;
  @override
  void write(Object? obj) => terminal.write('$obj');
  @override
  void writeAll(Iterable objects, [String separator = '']) =>
      write(objects.join(separator));
  @override
  void writeCharCode(int charCode) => write(String.fromCharCode(charCode));
  @override
  void writeln([Object? obj = '']) => write('$obj\n');
}

String redactDiff(String text) {
  if (RegExp(
    r'PRIVATE KEY|password|secret|token|api.?key|authorization|cookie',
    caseSensitive: false,
  ).hasMatch(text)) {
    return '[REDACTED]';
  }
  return text;
}
