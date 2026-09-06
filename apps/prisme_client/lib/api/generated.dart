// GENERATED from contracts/openapi.json. Do not edit.
// Regenerate: dart run tool/generate_api.dart
// ignore_for_file: non_constant_identifier_names, unused_local_variable

typedef ApiTransport =
    Future<Map<String, dynamic>> Function(
      String method,
      String path, {
      Map<String, dynamic>? body,
      String? idempotencyKey,
    });

class Status {
  const Status({required this.status});
  final String status;
  factory Status.fromJson(Map<String, dynamic> json) =>
      Status(status: json['status'] as String);
  Map<String, dynamic> toJson() => {'status': status};
}

class LoginRequest {
  const LoginRequest({required this.secret, required this.device_name});
  final String secret;
  final String device_name;
  factory LoginRequest.fromJson(Map<String, dynamic> json) => LoginRequest(
    secret: json['secret'] as String,
    device_name: json['device_name'] as String,
  );
  Map<String, dynamic> toJson() => {
    'secret': secret,
    'device_name': device_name,
  };
}

class Session {
  const Session({
    required this.access_token,
    required this.expires_at,
    required this.auth_mode,
  });
  final String access_token;
  final String expires_at;
  final String auth_mode;
  factory Session.fromJson(Map<String, dynamic> json) => Session(
    access_token: json['access_token'] as String,
    expires_at: json['expires_at'] as String,
    auth_mode: json['auth_mode'] as String,
  );
  Map<String, dynamic> toJson() => {
    'access_token': access_token,
    'expires_at': expires_at,
    'auth_mode': auth_mode,
  };
}

class Identity {
  const Identity({
    required this.user_id,
    required this.device_name,
    required this.expires_at,
    required this.auth_mode,
  });
  final String user_id;
  final String device_name;
  final String expires_at;
  final String auth_mode;
  factory Identity.fromJson(Map<String, dynamic> json) => Identity(
    user_id: json['user_id'] as String,
    device_name: json['device_name'] as String,
    expires_at: json['expires_at'] as String,
    auth_mode: json['auth_mode'] as String,
  );
  Map<String, dynamic> toJson() => {
    'user_id': user_id,
    'device_name': device_name,
    'expires_at': expires_at,
    'auth_mode': auth_mode,
  };
}

class Empty {
  const Empty();
  factory Empty.fromJson(Map<String, dynamic> json) => Empty();
  Map<String, dynamic> toJson() => {};
}

class Invitation {
  const Invitation({
    required this.token,
    required this.expires_at,
    required this.capabilities,
  });
  final String token;
  final String expires_at;
  final List<String> capabilities;
  factory Invitation.fromJson(Map<String, dynamic> json) => Invitation(
    token: json['token'] as String,
    expires_at: json['expires_at'] as String,
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'token': token,
    'expires_at': expires_at,
    'capabilities': capabilities,
  };
}

class EnrollRequest {
  const EnrollRequest({
    required this.token,
    required this.name,
    required this.csr_pem,
    required this.protocol_version,
  });
  final String token;
  final String name;
  final String csr_pem;
  final String protocol_version;
  factory EnrollRequest.fromJson(Map<String, dynamic> json) => EnrollRequest(
    token: json['token'] as String,
    name: json['name'] as String,
    csr_pem: json['csr_pem'] as String,
    protocol_version: json['protocol_version'] as String,
  );
  Map<String, dynamic> toJson() => {
    'token': token,
    'name': name,
    'csr_pem': csr_pem,
    'protocol_version': protocol_version,
  };
}

class Enrollment {
  const Enrollment({
    required this.agent_id,
    required this.certificate_pem,
    required this.expires_at,
    required this.protocol_version,
    required this.capabilities,
  });
  final String agent_id;
  final String certificate_pem;
  final String expires_at;
  final String protocol_version;
  final List<String> capabilities;
  factory Enrollment.fromJson(Map<String, dynamic> json) => Enrollment(
    agent_id: json['agent_id'] as String,
    certificate_pem: json['certificate_pem'] as String,
    expires_at: json['expires_at'] as String,
    protocol_version: json['protocol_version'] as String,
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'agent_id': agent_id,
    'certificate_pem': certificate_pem,
    'expires_at': expires_at,
    'protocol_version': protocol_version,
    'capabilities': capabilities,
  };
}

class HeartbeatRequest {
  const HeartbeatRequest({required this.protocol_version});
  final String protocol_version;
  factory HeartbeatRequest.fromJson(Map<String, dynamic> json) =>
      HeartbeatRequest(protocol_version: json['protocol_version'] as String);
  Map<String, dynamic> toJson() => {'protocol_version': protocol_version};
}

class Heartbeat {
  const Heartbeat({
    required this.agent_id,
    required this.observed_at,
    required this.capabilities,
  });
  final String agent_id;
  final String observed_at;
  final List<String> capabilities;
  factory Heartbeat.fromJson(Map<String, dynamic> json) => Heartbeat(
    agent_id: json['agent_id'] as String,
    observed_at: json['observed_at'] as String,
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'agent_id': agent_id,
    'observed_at': observed_at,
    'capabilities': capabilities,
  };
}

class Agent {
  const Agent({
    required this.id,
    required this.name,
    required this.enrolled_at,
    required this.last_seen_at,
    required this.expires_at,
    required this.revoked,
    required this.capabilities,
  });
  final String id;
  final String name;
  final String enrolled_at;
  final String? last_seen_at;
  final String expires_at;
  final bool revoked;
  final List<String> capabilities;
  factory Agent.fromJson(Map<String, dynamic> json) => Agent(
    id: json['id'] as String,
    name: json['name'] as String,
    enrolled_at: json['enrolled_at'] as String,
    last_seen_at: json['last_seen_at'] as String?,
    expires_at: json['expires_at'] as String,
    revoked: json['revoked'] as bool,
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'enrolled_at': enrolled_at,
    'last_seen_at': last_seen_at,
    'expires_at': expires_at,
    'revoked': revoked,
    'capabilities': capabilities,
  };
}

class AgentPage {
  const AgentPage({
    required this.items,
    required this.next_cursor,
    required this.observed_at,
  });
  final List<Agent> items;
  final String next_cursor;
  final String observed_at;
  factory AgentPage.fromJson(Map<String, dynamic> json) => AgentPage(
    items: (json['items'] as List)
        .map((v) => Agent.fromJson(v as Map<String, dynamic>))
        .toList(),
    next_cursor: json['next_cursor'] as String,
    observed_at: json['observed_at'] as String,
  );
  Map<String, dynamic> toJson() => {
    'items': items.map((v) => v.toJson()).toList(),
    'next_cursor': next_cursor,
    'observed_at': observed_at,
  };
}

class ApiProblem {
  const ApiProblem({
    required this.code,
    required this.message,
    required this.details,
    required this.recommended_action,
  });
  final String code;
  final String message;
  final Map<String, String> details;
  final String recommended_action;
  factory ApiProblem.fromJson(Map<String, dynamic> json) => ApiProblem(
    code: json['code'] as String,
    message: json['message'] as String,
    details: Map<String, String>.from(json['details'] as Map),
    recommended_action: json['recommended_action'] as String,
  );
  Map<String, dynamic> toJson() => {
    'code': code,
    'message': message,
    'details': details,
    'recommended_action': recommended_action,
  };
}

class PrismeApi {
  const PrismeApi(this.send);
  final ApiTransport send;
  Future<Status> health() async {
    final json = await send('GET', '/healthz');
    return Status.fromJson(json);
  }

  Future<Status> ready() async {
    final json = await send('GET', '/readyz');
    return Status.fromJson(json);
  }

  Future<Session> login(LoginRequest request) async {
    final json = await send('POST', '/v1/dev/sessions', body: request.toJson());
    return Session.fromJson(json);
  }

  Future<Identity> me() async {
    final json = await send('GET', '/v1/me');
    return Identity.fromJson(json);
  }

  Future<void> logout() async {
    final json = await send('DELETE', '/v1/session');
  }

  Future<AgentPage> agents({String cursor = ''}) async {
    final json = await send(
      'GET',
      '/v1/agents?cursor=${Uri.encodeQueryComponent(cursor)}',
    );
    return AgentPage.fromJson(json);
  }

  Future<Invitation> invite(
    Empty request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/enrollment-tokens',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return Invitation.fromJson(json);
  }

  Future<Enrollment> enroll(EnrollRequest request) async {
    final json = await send('POST', '/v1/agent/enroll', body: request.toJson());
    return Enrollment.fromJson(json);
  }

  Future<Heartbeat> heartbeat(HeartbeatRequest request) async {
    final json = await send(
      'POST',
      '/v1/agent/heartbeat',
      body: request.toJson(),
    );
    return Heartbeat.fromJson(json);
  }
}
