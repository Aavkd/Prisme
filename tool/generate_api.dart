// Deliberately bounded OpenAPI 3.1 projection for phase 0. Unsupported schemas fail.
import 'dart:convert';
import 'dart:io';

String refName(dynamic schema) => (schema[r'$ref'] as String).split('/').last;
String typeOf(Map<String, dynamic> schema) {
  if (schema.containsKey(r'$ref')) return refName(schema);
  final type = schema['type'];
  if (type is List) {
    if (type.length != 2 || !type.contains('null')) {
      throw UnsupportedError('Only nullable scalars are supported');
    }
    return '${typeOf({...schema, 'type': type.firstWhere((v) => v != 'null')})}?';
  }
  return switch (type) {
    'string' => 'String',
    'boolean' => 'bool',
    'integer' => 'int',
    'number' => 'num',
    'array' => 'List<${typeOf(schema['items'] as Map<String, dynamic>)}>',
    'object'
        when schema['additionalProperties'] is Map &&
            schema['additionalProperties']['type'] == 'string' =>
      'Map<String, String>',
    'object' => 'Map<String, dynamic>',
    _ => throw UnsupportedError('Unsupported schema: $schema'),
  };
}

String decode(Map<String, dynamic> schema, String value) {
  if (schema['type'] is List && (schema['type'] as List).contains('null')) {
    return '$value == null ? null : ${decode({...schema, 'type': (schema['type'] as List).firstWhere((v) => v != 'null')}, value)}';
  }
  if (schema.containsKey(r'$ref')) {
    return '${refName(schema)}.fromJson($value as Map<String, dynamic>)';
  }
  if (schema['type'] == 'array') {
    return '($value as List).map((v) => ${decode(schema['items'] as Map<String, dynamic>, 'v')}).toList()';
  }
  if (schema['type'] == 'object') {
    return '${typeOf(schema)}.from($value as Map)';
  }
  return '$value as ${typeOf(schema)}';
}

String encode(Map<String, dynamic> schema, String value) {
  if (schema.containsKey(r'$ref')) return '$value.toJson()';
  if (schema['type'] == 'array' && schema['items'][r'$ref'] != null) {
    return '$value.map((v) => v.toJson()).toList()';
  }
  return value;
}

Future<void> main(List<String> args) async {
  final root = File.fromUri(Platform.script).parent.parent.path;
  final spec =
      jsonDecode(File('$root/contracts/openapi.json').readAsStringSync())
          as Map<String, dynamic>;
  final out = StringBuffer(
    '''// GENERATED from contracts/openapi.json. Do not edit.
// Regenerate: dart run tool/generate_api.dart
// ignore_for_file: non_constant_identifier_names, unused_local_variable

typedef ApiTransport = Future<Map<String, dynamic>> Function(
  String method, String path, {Map<String, dynamic>? body, String? idempotencyKey});

''',
  );
  final schemas = spec['components']['schemas'] as Map<String, dynamic>;
  for (final entry in schemas.entries) {
    final schema = entry.value as Map<String, dynamic>;
    if (schema['x-preserve-json'] == true) {
      out.writeln('class ${entry.key} {');
      out.writeln('  const ${entry.key}(this.data);');
      out.writeln('  final Map<String, dynamic> data;');
      out.writeln(
        '  factory ${entry.key}.fromJson(Map<String, dynamic> json) => ${entry.key}(Map<String, dynamic>.from(json));',
      );
      out.writeln('  Map<String, dynamic> toJson() => data;');
      out.writeln('}');
      continue;
    }
    final originalProps = schema['properties'] as Map<String, dynamic>;
    final required = schema['required'] as List? ?? [];
    if (schema['type'] != 'object') {
      throw UnsupportedError('DTOs must be objects');
    }
    final props = <String, dynamic>{};
    for (final property in originalProps.entries) {
      final field = Map<String, dynamic>.from(property.value as Map);
      if (!required.contains(property.key)) {
        if (field.containsKey(r'$ref') || field['type'] is! String)
          throw UnsupportedError('Optional reference/union unsupported');
        field['type'] = [field['type'], 'null'];
      }
      props[property.key] = field;
    }
    out.writeln('class ${entry.key} {');
    out.writeln(
      props.isEmpty
          ? '  const ${entry.key}();'
          : '  const ${entry.key}({${props.keys.map((k) => '${required.contains(k) ? 'required ' : ''}this.$k').join(', ')}});',
    );
    for (final p in props.entries) {
      out.writeln(
        '  final ${typeOf(p.value as Map<String, dynamic>)} ${p.key};',
      );
    }
    out.writeln(
      '  factory ${entry.key}.fromJson(Map<String, dynamic> json) => ${entry.key}(',
    );
    for (final p in props.entries) {
      out.writeln(
        "    ${p.key}: ${decode(p.value as Map<String, dynamic>, "json['${p.key}']")},",
      );
    }
    out.writeln('  );\n  Map<String, dynamic> toJson() => {');
    for (final p in props.entries) {
      out.writeln(
        "    ${required.contains(p.key) ? '' : 'if (${p.key} != null) '}'${p.key}': ${encode(p.value as Map<String, dynamic>, p.key)},",
      );
    }
    out.writeln('  };\n}\n');
  }
  out.writeln(
    'class PrismeApi {\n  const PrismeApi(this.send);\n  final ApiTransport send;',
  );
  for (final path in (spec['paths'] as Map<String, dynamic>).entries) {
    for (final method in (path.value as Map<String, dynamic>).entries) {
      final op = method.value;
      if (op['x-browser-only'] == true || op['x-native-stream'] == true)
        continue;
      final input = op['requestBody']?['content']['application/json']['schema'];
      final response = (op['responses'] as Map).entries
          .firstWhere((e) => e.key.toString().startsWith('2'))
          .value;
      final output = response['content']?['application/json']['schema'];
      final parameters = op['parameters'] as List? ?? [];
      final args = <String>[if (input != null) '${refName(input)} request'];
      final named = <String>[];
      var expression = "'${path.key}'";
      var callArgs = input == null ? '' : ', body: request.toJson()';
      final queryParts = <String>[];
      for (final p in parameters) {
        if (p['in'] == 'query' && p['schema']['type'] == 'string') {
          final name = p['name'];
          named.add(
            p['required'] == true
                ? 'required String $name'
                : "String $name = ''",
          );
          queryParts.add('$name=\${Uri.encodeQueryComponent($name)}');
        } else if (p['name'] == 'Idempotency-Key' && p['in'] == 'header') {
          named.add('required String idempotencyKey');
          callArgs += ', idempotencyKey: idempotencyKey';
        } else if (p['in'] == 'path') {
          named.add('required String ${p['name']}');
          expression = expression.replaceAll(
            '{${p['name']}}',
            '\${Uri.encodeComponent(${p['name']})}',
          );
        } else {
          throw UnsupportedError('Unsupported parameter $p');
        }
      }
      if (queryParts.isNotEmpty) {
        expression =
            "${expression.substring(0, expression.length - 1)}?${queryParts.join('&')}'";
      }
      if (named.isNotEmpty) args.add('{${named.join(', ')}}');
      out.writeln(
        '  Future<${output == null ? 'void' : refName(output)}> ${op['operationId']}(${args.join(', ')}) async {',
      );
      out.writeln(
        "    final json = await send('${method.key.toUpperCase()}', $expression$callArgs);",
      );
      if (output != null)
        out.writeln('    return ${refName(output)}.fromJson(json);');
      out.writeln('  }');
    }
  }
  out.writeln('}');
  final target = File('$root/apps/prisme_client/lib/api/generated.dart');
  await target.parent.create(recursive: true);
  final temporary = File('${target.path}.tmp.dart');
  await temporary.writeAsString(out.toString());
  final format = await Process.run(Platform.resolvedExecutable, [
    'format',
    temporary.path,
  ]);
  if (format.exitCode != 0)
    throw StateError('Generated Dart formatting failed');
  final content = await temporary.readAsString();
  await temporary.delete();
  if (args.contains('--check')) {
    if (!target.existsSync() || target.readAsStringSync() != content) {
      stderr.writeln(
        'Generated API is stale. Run dart run tool/generate_api.dart.',
      );
      exitCode = 1;
    }
  } else {
    await target.writeAsString(content);
  }
}
