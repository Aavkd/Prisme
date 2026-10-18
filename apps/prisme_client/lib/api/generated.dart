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
  const LoginRequest({
    required this.secret,
    required this.device_name,
    this.device_id,
  });
  final String secret;
  final String device_name;
  final String? device_id;
  factory LoginRequest.fromJson(Map<String, dynamic> json) => LoginRequest(
    secret: json['secret'] as String,
    device_name: json['device_name'] as String,
    device_id: json['device_id'] == null ? null : json['device_id'] as String,
  );
  Map<String, dynamic> toJson() => {
    'secret': secret,
    'device_name': device_name,
    if (device_id != null) 'device_id': device_id,
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
    last_seen_at: json['last_seen_at'] == null
        ? null
        : json['last_seen_at'] as String,
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

class BrowserLoginRequest {
  const BrowserLoginRequest({
    required this.device_id,
    required this.device_name,
  });
  final String device_id;
  final String device_name;
  factory BrowserLoginRequest.fromJson(Map<String, dynamic> json) =>
      BrowserLoginRequest(
        device_id: json['device_id'] as String,
        device_name: json['device_name'] as String,
      );
  Map<String, dynamic> toJson() => {
    'device_id': device_id,
    'device_name': device_name,
  };
}

class BrowserLogin {
  const BrowserLogin({
    required this.flow_id,
    required this.poll_token,
    required this.browser_url,
    required this.expires_at,
  });
  final String flow_id;
  final String poll_token;
  final String browser_url;
  final String expires_at;
  factory BrowserLogin.fromJson(Map<String, dynamic> json) => BrowserLogin(
    flow_id: json['flow_id'] as String,
    poll_token: json['poll_token'] as String,
    browser_url: json['browser_url'] as String,
    expires_at: json['expires_at'] as String,
  );
  Map<String, dynamic> toJson() => {
    'flow_id': flow_id,
    'poll_token': poll_token,
    'browser_url': browser_url,
    'expires_at': expires_at,
  };
}

class PollRequest {
  const PollRequest({required this.flow_id, required this.poll_token});
  final String flow_id;
  final String poll_token;
  factory PollRequest.fromJson(Map<String, dynamic> json) => PollRequest(
    flow_id: json['flow_id'] as String,
    poll_token: json['poll_token'] as String,
  );
  Map<String, dynamic> toJson() => {
    'flow_id': flow_id,
    'poll_token': poll_token,
  };
}

class PollResult {
  const PollResult({
    required this.status,
    required this.access_token,
    required this.expires_at,
    required this.auth_mode,
  });
  final String status;
  final String access_token;
  final String expires_at;
  final String auth_mode;
  factory PollResult.fromJson(Map<String, dynamic> json) => PollResult(
    status: json['status'] as String,
    access_token: json['access_token'] as String,
    expires_at: json['expires_at'] as String,
    auth_mode: json['auth_mode'] as String,
  );
  Map<String, dynamic> toJson() => {
    'status': status,
    'access_token': access_token,
    'expires_at': expires_at,
    'auth_mode': auth_mode,
  };
}

class Capabilities {
  const Capabilities({required this.capabilities});
  final List<String> capabilities;
  factory Capabilities.fromJson(Map<String, dynamic> json) => Capabilities(
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
  );
  Map<String, dynamic> toJson() => {'capabilities': capabilities};
}

class ServerRecord {
  const ServerRecord({
    required this.id,
    required this.name,
    required this.last_seen_at,
    required this.expires_at,
    required this.revoked,
    required this.capabilities,
    required this.connection_state,
    required this.inventory,
  });
  final String id;
  final String name;
  final String? last_seen_at;
  final String expires_at;
  final bool revoked;
  final List<String> capabilities;
  final String connection_state;
  final Map<String, dynamic>? inventory;
  factory ServerRecord.fromJson(Map<String, dynamic> json) => ServerRecord(
    id: json['id'] as String,
    name: json['name'] as String,
    last_seen_at: json['last_seen_at'] == null
        ? null
        : json['last_seen_at'] as String,
    expires_at: json['expires_at'] as String,
    revoked: json['revoked'] as bool,
    capabilities: (json['capabilities'] as List)
        .map((v) => v as String)
        .toList(),
    connection_state: json['connection_state'] as String,
    inventory: json['inventory'] == null
        ? null
        : Map<String, dynamic>.from(json['inventory'] as Map),
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'last_seen_at': last_seen_at,
    'expires_at': expires_at,
    'revoked': revoked,
    'capabilities': capabilities,
    'connection_state': connection_state,
    'inventory': inventory,
  };
}

class ServerPage {
  const ServerPage({
    required this.items,
    required this.next_cursor,
    required this.observed_at,
  });
  final List<ServerRecord> items;
  final String next_cursor;
  final String observed_at;
  factory ServerPage.fromJson(Map<String, dynamic> json) => ServerPage(
    items: (json['items'] as List)
        .map((v) => ServerRecord.fromJson(v as Map<String, dynamic>))
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

class DeviceRecord {
  const DeviceRecord({
    required this.id,
    required this.name,
    required this.revoked,
    required this.created_at,
    required this.current,
  });
  final String id;
  final String name;
  final bool revoked;
  final String created_at;
  final bool current;
  factory DeviceRecord.fromJson(Map<String, dynamic> json) => DeviceRecord(
    id: json['id'] as String,
    name: json['name'] as String,
    revoked: json['revoked'] as bool,
    created_at: json['created_at'] as String,
    current: json['current'] as bool,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'revoked': revoked,
    'created_at': created_at,
    'current': current,
  };
}

class DevicePage {
  const DevicePage({required this.items});
  final List<DeviceRecord> items;
  factory DevicePage.fromJson(Map<String, dynamic> json) => DevicePage(
    items: (json['items'] as List)
        .map((v) => DeviceRecord.fromJson(v as Map<String, dynamic>))
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'items': items.map((v) => v.toJson()).toList(),
  };
}

class DevicePreferences {
  const DevicePreferences({
    required this.pinned_servers,
    required this.pinned_workers,
  });
  final List<String> pinned_servers;
  final List<String> pinned_workers;
  factory DevicePreferences.fromJson(Map<String, dynamic> json) =>
      DevicePreferences(
        pinned_servers: (json['pinned_servers'] as List)
            .map((v) => v as String)
            .toList(),
        pinned_workers: (json['pinned_workers'] as List)
            .map((v) => v as String)
            .toList(),
      );
  Map<String, dynamic> toJson() => {
    'pinned_servers': pinned_servers,
    'pinned_workers': pinned_workers,
  };
}

class RecoveryCodes {
  const RecoveryCodes({required this.codes});
  final List<String> codes;
  factory RecoveryCodes.fromJson(Map<String, dynamic> json) => RecoveryCodes(
    codes: (json['codes'] as List).map((v) => v as String).toList(),
  );
  Map<String, dynamic> toJson() => {'codes': codes};
}

class RegistrationTicket {
  const RegistrationTicket({required this.ticket, required this.expires_at});
  final String ticket;
  final String expires_at;
  factory RegistrationTicket.fromJson(Map<String, dynamic> json) =>
      RegistrationTicket(
        ticket: json['ticket'] as String,
        expires_at: json['expires_at'] as String,
      );
  Map<String, dynamic> toJson() => {'ticket': ticket, 'expires_at': expires_at};
}

class AuditEvent {
  const AuditEvent({
    required this.id,
    required this.occurred_at,
    required this.actor,
    required this.action,
    required this.target,
    required this.device_id,
    required this.origin,
    required this.result,
    required this.correlation_id,
    required this.environment,
    required this.remote_address,
  });
  final String id;
  final String occurred_at;
  final String actor;
  final String action;
  final String target;
  final String device_id;
  final String origin;
  final String result;
  final String correlation_id;
  final String environment;
  final String remote_address;
  factory AuditEvent.fromJson(Map<String, dynamic> json) => AuditEvent(
    id: json['id'] as String,
    occurred_at: json['occurred_at'] as String,
    actor: json['actor'] as String,
    action: json['action'] as String,
    target: json['target'] as String,
    device_id: json['device_id'] as String,
    origin: json['origin'] as String,
    result: json['result'] as String,
    correlation_id: json['correlation_id'] as String,
    environment: json['environment'] as String,
    remote_address: json['remote_address'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'occurred_at': occurred_at,
    'actor': actor,
    'action': action,
    'target': target,
    'device_id': device_id,
    'origin': origin,
    'result': result,
    'correlation_id': correlation_id,
    'environment': environment,
    'remote_address': remote_address,
  };
}

class AuditPage {
  const AuditPage({required this.items, required this.next_cursor});
  final List<AuditEvent> items;
  final String next_cursor;
  factory AuditPage.fromJson(Map<String, dynamic> json) => AuditPage(
    items: (json['items'] as List)
        .map((v) => AuditEvent.fromJson(v as Map<String, dynamic>))
        .toList(),
    next_cursor: json['next_cursor'] as String,
  );
  Map<String, dynamic> toJson() => {
    'items': items.map((v) => v.toJson()).toList(),
    'next_cursor': next_cursor,
  };
}

class RenewalRequest {
  const RenewalRequest({required this.csr_pem});
  final String csr_pem;
  factory RenewalRequest.fromJson(Map<String, dynamic> json) =>
      RenewalRequest(csr_pem: json['csr_pem'] as String);
  Map<String, dynamic> toJson() => {'csr_pem': csr_pem};
}

class InventoryHost {
  const InventoryHost({
    required this.hostname,
    required this.os,
    required this.architecture,
    required this.cpus,
    required this.memory_bytes,
    required this.disk_total_bytes,
    required this.disk_free_bytes,
  });
  final String hostname;
  final String os;
  final String architecture;
  final int cpus;
  final int? memory_bytes;
  final int? disk_total_bytes;
  final int? disk_free_bytes;
  factory InventoryHost.fromJson(Map<String, dynamic> json) => InventoryHost(
    hostname: json['hostname'] as String,
    os: json['os'] as String,
    architecture: json['architecture'] as String,
    cpus: json['cpus'] as int,
    memory_bytes: json['memory_bytes'] == null
        ? null
        : json['memory_bytes'] as int,
    disk_total_bytes: json['disk_total_bytes'] == null
        ? null
        : json['disk_total_bytes'] as int,
    disk_free_bytes: json['disk_free_bytes'] == null
        ? null
        : json['disk_free_bytes'] as int,
  );
  Map<String, dynamic> toJson() => {
    'hostname': hostname,
    'os': os,
    'architecture': architecture,
    'cpus': cpus,
    'memory_bytes': memory_bytes,
    'disk_total_bytes': disk_total_bytes,
    'disk_free_bytes': disk_free_bytes,
  };
}

class InventoryContainer {
  const InventoryContainer({
    required this.id,
    required this.name,
    required this.image,
    required this.image_id,
    required this.state,
    required this.health,
    required this.project,
    required this.service,
    required this.config_files,
    required this.ports,
    required this.networks,
  });
  final String id;
  final String name;
  final String image;
  final String image_id;
  final String state;
  final String health;
  final String project;
  final String service;
  final String config_files;
  final String ports;
  final String networks;
  factory InventoryContainer.fromJson(Map<String, dynamic> json) =>
      InventoryContainer(
        id: json['id'] as String,
        name: json['name'] as String,
        image: json['image'] as String,
        image_id: json['image_id'] as String,
        state: json['state'] as String,
        health: json['health'] as String,
        project: json['project'] as String,
        service: json['service'] as String,
        config_files: json['config_files'] as String,
        ports: json['ports'] as String,
        networks: json['networks'] as String,
      );
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'image': image,
    'image_id': image_id,
    'state': state,
    'health': health,
    'project': project,
    'service': service,
    'config_files': config_files,
    'ports': ports,
    'networks': networks,
  };
}

class InventoryResource {
  const InventoryResource({
    required this.name,
    required this.driver,
    required this.project,
  });
  final String name;
  final String driver;
  final String project;
  factory InventoryResource.fromJson(Map<String, dynamic> json) =>
      InventoryResource(
        name: json['name'] as String,
        driver: json['driver'] as String,
        project: json['project'] as String,
      );
  Map<String, dynamic> toJson() => {
    'name': name,
    'driver': driver,
    'project': project,
  };
}

class InventoryImage {
  const InventoryImage({
    required this.id,
    required this.tags,
    required this.digests,
  });
  final String id;
  final List<String> tags;
  final List<String> digests;
  factory InventoryImage.fromJson(Map<String, dynamic> json) => InventoryImage(
    id: json['id'] as String,
    tags: (json['tags'] as List).map((v) => v as String).toList(),
    digests: (json['digests'] as List).map((v) => v as String).toList(),
  );
  Map<String, dynamic> toJson() => {'id': id, 'tags': tags, 'digests': digests};
}

class InventorySnapshot {
  const InventorySnapshot({
    required this.protocol_version,
    required this.observed_at,
    required this.source,
    required this.host,
    required this.docker_available,
    required this.containers,
    required this.networks,
    required this.volumes,
    required this.images,
    required this.warnings,
  });
  final String protocol_version;
  final String observed_at;
  final String source;
  final InventoryHost host;
  final bool docker_available;
  final List<InventoryContainer> containers;
  final List<InventoryResource> networks;
  final List<InventoryResource> volumes;
  final List<InventoryImage> images;
  final List<String> warnings;
  factory InventorySnapshot.fromJson(Map<String, dynamic> json) =>
      InventorySnapshot(
        protocol_version: json['protocol_version'] as String,
        observed_at: json['observed_at'] as String,
        source: json['source'] as String,
        host: InventoryHost.fromJson(json['host'] as Map<String, dynamic>),
        docker_available: json['docker_available'] as bool,
        containers: (json['containers'] as List)
            .map((v) => InventoryContainer.fromJson(v as Map<String, dynamic>))
            .toList(),
        networks: (json['networks'] as List)
            .map((v) => InventoryResource.fromJson(v as Map<String, dynamic>))
            .toList(),
        volumes: (json['volumes'] as List)
            .map((v) => InventoryResource.fromJson(v as Map<String, dynamic>))
            .toList(),
        images: (json['images'] as List)
            .map((v) => InventoryImage.fromJson(v as Map<String, dynamic>))
            .toList(),
        warnings: (json['warnings'] as List).map((v) => v as String).toList(),
      );
  Map<String, dynamic> toJson() => {
    'protocol_version': protocol_version,
    'observed_at': observed_at,
    'source': source,
    'host': host.toJson(),
    'docker_available': docker_available,
    'containers': containers.map((v) => v.toJson()).toList(),
    'networks': networks.map((v) => v.toJson()).toList(),
    'volumes': volumes.map((v) => v.toJson()).toList(),
    'images': images.map((v) => v.toJson()).toList(),
    'warnings': warnings,
  };
}

class ImportRequest {
  const ImportRequest({required this.name, required this.snapshot});
  final String name;
  final InventorySnapshot snapshot;
  factory ImportRequest.fromJson(Map<String, dynamic> json) => ImportRequest(
    name: json['name'] as String,
    snapshot: InventorySnapshot.fromJson(
      json['snapshot'] as Map<String, dynamic>,
    ),
  );
  Map<String, dynamic> toJson() => {
    'name': name,
    'snapshot': snapshot.toJson(),
  };
}

class ImportResult {
  const ImportResult({required this.server_id});
  final String server_id;
  factory ImportResult.fromJson(Map<String, dynamic> json) =>
      ImportResult(server_id: json['server_id'] as String);
  Map<String, dynamic> toJson() => {'server_id': server_id};
}

class Accepted {
  const Accepted({required this.status});
  final String status;
  factory Accepted.fromJson(Map<String, dynamic> json) =>
      Accepted(status: json['status'] as String);
  Map<String, dynamic> toJson() => {'status': status};
}

class PasskeyBeginRequest {
  const PasskeyBeginRequest({
    required this.flow_id,
    required this.kind,
    required this.authorization,
  });
  final String flow_id;
  final String kind;
  final String authorization;
  factory PasskeyBeginRequest.fromJson(Map<String, dynamic> json) =>
      PasskeyBeginRequest(
        flow_id: json['flow_id'] as String,
        kind: json['kind'] as String,
        authorization: json['authorization'] as String,
      );
  Map<String, dynamic> toJson() => {
    'flow_id': flow_id,
    'kind': kind,
    'authorization': authorization,
  };
}

class PasskeyBegin {
  const PasskeyBegin({
    required this.ceremony_id,
    required this.options,
    required this.device_name,
  });
  final String ceremony_id;
  final Map<String, dynamic> options;
  final String device_name;
  factory PasskeyBegin.fromJson(Map<String, dynamic> json) => PasskeyBegin(
    ceremony_id: json['ceremony_id'] as String,
    options: Map<String, dynamic>.from(json['options'] as Map),
    device_name: json['device_name'] as String,
  );
  Map<String, dynamic> toJson() => {
    'ceremony_id': ceremony_id,
    'options': options,
    'device_name': device_name,
  };
}

class PasskeyFinish {
  const PasskeyFinish({required this.ceremony_id, required this.credential});
  final String ceremony_id;
  final Map<String, dynamic> credential;
  factory PasskeyFinish.fromJson(Map<String, dynamic> json) => PasskeyFinish(
    ceremony_id: json['ceremony_id'] as String,
    credential: Map<String, dynamic>.from(json['credential'] as Map),
  );
  Map<String, dynamic> toJson() => {
    'ceremony_id': ceremony_id,
    'credential': credential,
  };
}

class ObservationScope {
  const ObservationScope({
    required this.id,
    required this.server,
    required this.project,
    required this.environment,
    required this.service,
  });
  final String id;
  final String server;
  final String project;
  final String environment;
  final String service;
  factory ObservationScope.fromJson(Map<String, dynamic> json) =>
      ObservationScope(
        id: json['id'] as String,
        server: json['server'] as String,
        project: json['project'] as String,
        environment: json['environment'] as String,
        service: json['service'] as String,
      );
  Map<String, dynamic> toJson() => {
    'id': id,
    'server': server,
    'project': project,
    'environment': environment,
    'service': service,
  };
}

class ObservationRule {
  const ObservationRule({required this.definition, required this.state});
  final Map<String, dynamic> definition;
  final Map<String, dynamic> state;
  factory ObservationRule.fromJson(Map<String, dynamic> json) =>
      ObservationRule(
        definition: Map<String, dynamic>.from(json['definition'] as Map),
        state: Map<String, dynamic>.from(json['state'] as Map),
      );
  Map<String, dynamic> toJson() => {'definition': definition, 'state': state};
}

class ObservationOverview {
  const ObservationOverview({
    required this.enabled,
    required this.scopes,
    required this.rules,
    required this.observed_at,
  });
  final bool enabled;
  final List<ObservationScope> scopes;
  final List<ObservationRule> rules;
  final String observed_at;
  factory ObservationOverview.fromJson(Map<String, dynamic> json) =>
      ObservationOverview(
        enabled: json['enabled'] as bool,
        scopes: (json['scopes'] as List)
            .map((v) => ObservationScope.fromJson(v as Map<String, dynamic>))
            .toList(),
        rules: (json['rules'] as List)
            .map((v) => ObservationRule.fromJson(v as Map<String, dynamic>))
            .toList(),
        observed_at: json['observed_at'] as String,
      );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'scopes': scopes.map((v) => v.toJson()).toList(),
    'rules': rules.map((v) => v.toJson()).toList(),
    'observed_at': observed_at,
  };
}

class MetricPoint {
  const MetricPoint({required this.at, required this.value});
  final String at;
  final num? value;
  factory MetricPoint.fromJson(Map<String, dynamic> json) => MetricPoint(
    at: json['at'] as String,
    value: json['value'] == null ? null : json['value'] as num,
  );
  Map<String, dynamic> toJson() => {'at': at, 'value': value};
}

class MetricSeries {
  const MetricSeries({required this.labels, required this.points});
  final Map<String, String> labels;
  final List<MetricPoint> points;
  factory MetricSeries.fromJson(Map<String, dynamic> json) => MetricSeries(
    labels: Map<String, String>.from(json['labels'] as Map),
    points: (json['points'] as List)
        .map((v) => MetricPoint.fromJson(v as Map<String, dynamic>))
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'labels': labels,
    'points': points.map((v) => v.toJson()).toList(),
  };
}

class MetricResult {
  const MetricResult({
    required this.series,
    required this.start,
    required this.end,
    required this.observed_at,
    required this.source,
    required this.step_seconds,
  });
  final List<MetricSeries> series;
  final String start;
  final String end;
  final String observed_at;
  final String source;
  final int step_seconds;
  factory MetricResult.fromJson(Map<String, dynamic> json) => MetricResult(
    series: (json['series'] as List)
        .map((v) => MetricSeries.fromJson(v as Map<String, dynamic>))
        .toList(),
    start: json['start'] as String,
    end: json['end'] as String,
    observed_at: json['observed_at'] as String,
    source: json['source'] as String,
    step_seconds: json['step_seconds'] as int,
  );
  Map<String, dynamic> toJson() => {
    'series': series.map((v) => v.toJson()).toList(),
    'start': start,
    'end': end,
    'observed_at': observed_at,
    'source': source,
    'step_seconds': step_seconds,
  };
}

class ObservationLog {
  const ObservationLog({
    required this.at,
    required this.line,
    required this.labels,
  });
  final String at;
  final String line;
  final Map<String, String> labels;
  factory ObservationLog.fromJson(Map<String, dynamic> json) => ObservationLog(
    at: json['at'] as String,
    line: json['line'] as String,
    labels: Map<String, String>.from(json['labels'] as Map),
  );
  Map<String, dynamic> toJson() => {'at': at, 'line': line, 'labels': labels};
}

class LogResult {
  const LogResult({
    required this.lines,
    required this.truncated,
    required this.start,
    required this.end,
    required this.observed_at,
    required this.source,
  });
  final List<ObservationLog> lines;
  final bool truncated;
  final String start;
  final String end;
  final String observed_at;
  final String source;
  factory LogResult.fromJson(Map<String, dynamic> json) => LogResult(
    lines: (json['lines'] as List)
        .map((v) => ObservationLog.fromJson(v as Map<String, dynamic>))
        .toList(),
    truncated: json['truncated'] as bool,
    start: json['start'] as String,
    end: json['end'] as String,
    observed_at: json['observed_at'] as String,
    source: json['source'] as String,
  );
  Map<String, dynamic> toJson() => {
    'lines': lines.map((v) => v.toJson()).toList(),
    'truncated': truncated,
    'start': start,
    'end': end,
    'observed_at': observed_at,
    'source': source,
  };
}

class IncidentRecord {
  const IncidentRecord({
    required this.id,
    required this.rule_id,
    required this.scope_id,
    required this.title,
    required this.severity,
    required this.status,
    required this.opened_at,
    required this.updated_at,
    required this.resolved_at,
    required this.revision,
  });
  final String id;
  final String rule_id;
  final String scope_id;
  final String title;
  final String severity;
  final String status;
  final String opened_at;
  final String updated_at;
  final String? resolved_at;
  final int revision;
  factory IncidentRecord.fromJson(Map<String, dynamic> json) => IncidentRecord(
    id: json['id'] as String,
    rule_id: json['rule_id'] as String,
    scope_id: json['scope_id'] as String,
    title: json['title'] as String,
    severity: json['severity'] as String,
    status: json['status'] as String,
    opened_at: json['opened_at'] as String,
    updated_at: json['updated_at'] as String,
    resolved_at: json['resolved_at'] == null
        ? null
        : json['resolved_at'] as String,
    revision: json['revision'] as int,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'rule_id': rule_id,
    'scope_id': scope_id,
    'title': title,
    'severity': severity,
    'status': status,
    'opened_at': opened_at,
    'updated_at': updated_at,
    'resolved_at': resolved_at,
    'revision': revision,
  };
}

class IncidentEvent {
  const IncidentEvent({
    required this.id,
    required this.at,
    required this.kind,
    required this.actor,
    required this.message,
  });
  final String id;
  final String at;
  final String kind;
  final String actor;
  final String message;
  factory IncidentEvent.fromJson(Map<String, dynamic> json) => IncidentEvent(
    id: json['id'] as String,
    at: json['at'] as String,
    kind: json['kind'] as String,
    actor: json['actor'] as String,
    message: json['message'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'at': at,
    'kind': kind,
    'actor': actor,
    'message': message,
  };
}

class IncidentPage {
  const IncidentPage({
    required this.items,
    required this.next_cursor,
    required this.observed_at,
  });
  final List<IncidentRecord> items;
  final String next_cursor;
  final String observed_at;
  factory IncidentPage.fromJson(Map<String, dynamic> json) => IncidentPage(
    items: (json['items'] as List)
        .map((v) => IncidentRecord.fromJson(v as Map<String, dynamic>))
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

class IncidentDetail {
  const IncidentDetail({
    required this.incident,
    required this.events,
    required this.next_cursor,
    required this.observed_at,
  });
  final IncidentRecord incident;
  final List<IncidentEvent> events;
  final String next_cursor;
  final String observed_at;
  factory IncidentDetail.fromJson(Map<String, dynamic> json) => IncidentDetail(
    incident: IncidentRecord.fromJson(json['incident'] as Map<String, dynamic>),
    events: (json['events'] as List)
        .map((v) => IncidentEvent.fromJson(v as Map<String, dynamic>))
        .toList(),
    next_cursor: json['next_cursor'] as String,
    observed_at: json['observed_at'] as String,
  );
  Map<String, dynamic> toJson() => {
    'incident': incident.toJson(),
    'events': events.map((v) => v.toJson()).toList(),
    'next_cursor': next_cursor,
    'observed_at': observed_at,
  };
}

class IncidentAction {
  const IncidentAction({
    required this.action,
    required this.note,
    required this.revision,
  });
  final String action;
  final String note;
  final int revision;
  factory IncidentAction.fromJson(Map<String, dynamic> json) => IncidentAction(
    action: json['action'] as String,
    note: json['note'] as String,
    revision: json['revision'] as int,
  );
  Map<String, dynamic> toJson() => {
    'action': action,
    'note': note,
    'revision': revision,
  };
}

class IncidentActionResult {
  const IncidentActionResult({required this.status, required this.revision});
  final String status;
  final int revision;
  factory IncidentActionResult.fromJson(Map<String, dynamic> json) =>
      IncidentActionResult(
        status: json['status'] as String,
        revision: json['revision'] as int,
      );
  Map<String, dynamic> toJson() => {'status': status, 'revision': revision};
}

class GithubRepository {
  const GithubRepository({
    required this.id,
    required this.name,
    required this.environment,
    required this.workflow_id,
    required this.error_code,
    required this.last_sync,
  });
  final String id;
  final String name;
  final String environment;
  final String workflow_id;
  final String error_code;
  final String? last_sync;
  factory GithubRepository.fromJson(Map<String, dynamic> json) =>
      GithubRepository(
        id: json['id'] as String,
        name: json['name'] as String,
        environment: json['environment'] as String,
        workflow_id: json['workflow_id'] as String,
        error_code: json['error_code'] as String,
        last_sync: json['last_sync'] == null
            ? null
            : json['last_sync'] as String,
      );
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'environment': environment,
    'workflow_id': workflow_id,
    'error_code': error_code,
    'last_sync': last_sync,
  };
}

class GithubOverview {
  const GithubOverview({required this.enabled, required this.repositories});
  final bool enabled;
  final List<GithubRepository> repositories;
  factory GithubOverview.fromJson(Map<String, dynamic> json) => GithubOverview(
    enabled: json['enabled'] as bool,
    repositories: (json['repositories'] as List)
        .map((v) => GithubRepository.fromJson(v as Map<String, dynamic>))
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'repositories': repositories.map((v) => v.toJson()).toList(),
  };
}

class GithubBuild {
  const GithubBuild({
    required this.id,
    required this.repository_id,
    required this.workflow_id,
    required this.name,
    required this.commit,
    required this.branch,
    required this.author,
    required this.status,
    required this.conclusion,
    required this.release_state,
    required this.attempt,
    required this.created_at,
    required this.updated_at,
    required this.started_at,
  });
  final String id;
  final String repository_id;
  final String workflow_id;
  final String name;
  final String commit;
  final String branch;
  final String author;
  final String status;
  final String conclusion;
  final String release_state;
  final int attempt;
  final String created_at;
  final String updated_at;
  final String? started_at;
  factory GithubBuild.fromJson(Map<String, dynamic> json) => GithubBuild(
    id: json['id'] as String,
    repository_id: json['repository_id'] as String,
    workflow_id: json['workflow_id'] as String,
    name: json['name'] as String,
    commit: json['commit'] as String,
    branch: json['branch'] as String,
    author: json['author'] as String,
    status: json['status'] as String,
    conclusion: json['conclusion'] as String,
    release_state: json['release_state'] as String,
    attempt: json['attempt'] as int,
    created_at: json['created_at'] as String,
    updated_at: json['updated_at'] as String,
    started_at: json['started_at'] == null
        ? null
        : json['started_at'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'repository_id': repository_id,
    'workflow_id': workflow_id,
    'name': name,
    'commit': commit,
    'branch': branch,
    'author': author,
    'status': status,
    'conclusion': conclusion,
    'release_state': release_state,
    'attempt': attempt,
    'created_at': created_at,
    'updated_at': updated_at,
    'started_at': started_at,
  };
}

class GithubBuildPage {
  const GithubBuildPage({required this.items, required this.has_more});
  final List<GithubBuild> items;
  final bool has_more;
  factory GithubBuildPage.fromJson(Map<String, dynamic> json) =>
      GithubBuildPage(
        items: (json['items'] as List)
            .map((v) => GithubBuild.fromJson(v as Map<String, dynamic>))
            .toList(),
        has_more: json['has_more'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'items': items.map((v) => v.toJson()).toList(),
    'has_more': has_more,
  };
}

class ReleaseImage {
  const ReleaseImage({
    required this.service,
    required this.reference,
    required this.tag,
    required this.provenance,
  });
  final String service;
  final String reference;
  final String tag;
  final String provenance;
  factory ReleaseImage.fromJson(Map<String, dynamic> json) => ReleaseImage(
    service: json['service'] as String,
    reference: json['reference'] as String,
    tag: json['tag'] as String,
    provenance: json['provenance'] as String,
  );
  Map<String, dynamic> toJson() => {
    'service': service,
    'reference': reference,
    'tag': tag,
    'provenance': provenance,
  };
}

class ReleaseManifest {
  const ReleaseManifest({
    required this.version,
    required this.repository,
    required this.commit,
    required this.run_id,
    required this.environment,
    required this.run_attempt,
    required this.images,
  });
  final int version;
  final String repository;
  final String commit;
  final String run_id;
  final String environment;
  final int run_attempt;
  final List<ReleaseImage> images;
  factory ReleaseManifest.fromJson(Map<String, dynamic> json) =>
      ReleaseManifest(
        version: json['version'] as int,
        repository: json['repository'] as String,
        commit: json['commit'] as String,
        run_id: json['run_id'] as String,
        environment: json['environment'] as String,
        run_attempt: json['run_attempt'] as int,
        images: (json['images'] as List)
            .map((v) => ReleaseImage.fromJson(v as Map<String, dynamic>))
            .toList(),
      );
  Map<String, dynamic> toJson() => {
    'version': version,
    'repository': repository,
    'commit': commit,
    'run_id': run_id,
    'environment': environment,
    'run_attempt': run_attempt,
    'images': images.map((v) => v.toJson()).toList(),
  };
}

class GithubRelease {
  const GithubRelease({
    required this.id,
    required this.repository_id,
    required this.artifact_id,
    required this.manifest,
  });
  final String id;
  final String repository_id;
  final String artifact_id;
  final ReleaseManifest manifest;
  factory GithubRelease.fromJson(Map<String, dynamic> json) => GithubRelease(
    id: json['id'] as String,
    repository_id: json['repository_id'] as String,
    artifact_id: json['artifact_id'] as String,
    manifest: ReleaseManifest.fromJson(
      json['manifest'] as Map<String, dynamic>,
    ),
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'repository_id': repository_id,
    'artifact_id': artifact_id,
    'manifest': manifest.toJson(),
  };
}

class GithubReleasePage {
  const GithubReleasePage({required this.items, required this.has_more});
  final List<GithubRelease> items;
  final bool has_more;
  factory GithubReleasePage.fromJson(Map<String, dynamic> json) =>
      GithubReleasePage(
        items: (json['items'] as List)
            .map((v) => GithubRelease.fromJson(v as Map<String, dynamic>))
            .toList(),
        has_more: json['has_more'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'items': items.map((v) => v.toJson()).toList(),
    'has_more': has_more,
  };
}

class GithubData {
  const GithubData({required this.data});
  final Map<String, dynamic> data;
  factory GithubData.fromJson(Map<String, dynamic> json) =>
      GithubData(data: Map<String, dynamic>.from(json['data'] as Map));
  Map<String, dynamic> toJson() => {'data': data};
}

class GithubLog {
  const GithubLog({required this.text});
  final String text;
  factory GithubLog.fromJson(Map<String, dynamic> json) =>
      GithubLog(text: json['text'] as String);
  Map<String, dynamic> toJson() => {'text': text};
}

class DeploymentJob {
  const DeploymentJob({
    required this.id,
    required this.target_id,
    required this.plan,
    required this.state,
    required this.revision,
    required this.step,
    required this.created_at,
    required this.expires_at,
    required this.heartbeat_at,
    required this.approved_by,
    required this.error_code,
  });
  final String id;
  final String target_id;
  final Map<String, dynamic> plan;
  final String state;
  final int revision;
  final int step;
  final String created_at;
  final String expires_at;
  final String? heartbeat_at;
  final String approved_by;
  final String error_code;
  factory DeploymentJob.fromJson(Map<String, dynamic> json) => DeploymentJob(
    id: json['id'] as String,
    target_id: json['target_id'] as String,
    plan: Map<String, dynamic>.from(json['plan'] as Map),
    state: json['state'] as String,
    revision: json['revision'] as int,
    step: json['step'] as int,
    created_at: json['created_at'] as String,
    expires_at: json['expires_at'] as String,
    heartbeat_at: json['heartbeat_at'] == null
        ? null
        : json['heartbeat_at'] as String,
    approved_by: json['approved_by'] as String,
    error_code: json['error_code'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'target_id': target_id,
    'plan': plan,
    'state': state,
    'revision': revision,
    'step': step,
    'created_at': created_at,
    'expires_at': expires_at,
    'heartbeat_at': heartbeat_at,
    'approved_by': approved_by,
    'error_code': error_code,
  };
}

class DeploymentOverview {
  const DeploymentOverview({
    required this.enabled,
    required this.targets,
    required this.items,
    required this.has_more,
  });
  final bool enabled;
  final List<Map<String, dynamic>> targets;
  final List<DeploymentJob> items;
  final bool has_more;
  factory DeploymentOverview.fromJson(Map<String, dynamic> json) =>
      DeploymentOverview(
        enabled: json['enabled'] as bool,
        targets: (json['targets'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
        items: (json['items'] as List)
            .map((v) => DeploymentJob.fromJson(v as Map<String, dynamic>))
            .toList(),
        has_more: json['has_more'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'targets': targets,
    'items': items.map((v) => v.toJson()).toList(),
    'has_more': has_more,
  };
}

class DeploymentPrepared {
  const DeploymentPrepared({required this.job});
  final DeploymentJob job;
  factory DeploymentPrepared.fromJson(Map<String, dynamic> json) =>
      DeploymentPrepared(
        job: DeploymentJob.fromJson(json['job'] as Map<String, dynamic>),
      );
  Map<String, dynamic> toJson() => {'job': job.toJson()};
}

class DeploymentDetail {
  const DeploymentDetail({required this.job, required this.events});
  final DeploymentJob job;
  final List<Map<String, dynamic>> events;
  factory DeploymentDetail.fromJson(Map<String, dynamic> json) =>
      DeploymentDetail(
        job: DeploymentJob.fromJson(json['job'] as Map<String, dynamic>),
        events: (json['events'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
      );
  Map<String, dynamic> toJson() => {'job': job.toJson(), 'events': events};
}

class DeploymentRequest {
  const DeploymentRequest({
    required this.target,
    required this.release,
    required this.rollback,
  });
  final String target;
  final String release;
  final bool rollback;
  factory DeploymentRequest.fromJson(Map<String, dynamic> json) =>
      DeploymentRequest(
        target: json['target'] as String,
        release: json['release'] as String,
        rollback: json['rollback'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'target': target,
    'release': release,
    'rollback': rollback,
  };
}

class DeploymentDecision {
  const DeploymentDecision({
    required this.action,
    required this.revision,
    required this.phrase,
    this.proof,
  });
  final String action;
  final int revision;
  final String phrase;
  final String? proof;
  factory DeploymentDecision.fromJson(Map<String, dynamic> json) =>
      DeploymentDecision(
        action: json['action'] as String,
        revision: json['revision'] as int,
        phrase: json['phrase'] as String,
        proof: json['proof'] == null ? null : json['proof'] as String,
      );
  Map<String, dynamic> toJson() => {
    'action': action,
    'revision': revision,
    'phrase': phrase,
    if (proof != null) 'proof': proof,
  };
}

class NotificationPreferences {
  const NotificationPreferences({
    required this.enabled,
    required this.timezone,
    required this.quiet_start,
    required this.quiet_end,
  });
  final bool enabled;
  final String timezone;
  final int quiet_start;
  final int quiet_end;
  factory NotificationPreferences.fromJson(Map<String, dynamic> json) =>
      NotificationPreferences(
        enabled: json['enabled'] as bool,
        timezone: json['timezone'] as String,
        quiet_start: json['quiet_start'] as int,
        quiet_end: json['quiet_end'] as int,
      );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'timezone': timezone,
    'quiet_start': quiet_start,
    'quiet_end': quiet_end,
  };
}

class NotificationSettings {
  const NotificationSettings({
    required this.preferences,
    required this.configured,
    required this.action_key_registered,
  });
  final NotificationPreferences preferences;
  final bool configured;
  final bool action_key_registered;
  factory NotificationSettings.fromJson(Map<String, dynamic> json) =>
      NotificationSettings(
        preferences: NotificationPreferences.fromJson(
          json['preferences'] as Map<String, dynamic>,
        ),
        configured: json['configured'] as bool,
        action_key_registered: json['action_key_registered'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'preferences': preferences.toJson(),
    'configured': configured,
    'action_key_registered': action_key_registered,
  };
}

class NotificationSettingsRequest {
  const NotificationSettingsRequest({
    this.token,
    required this.enabled,
    required this.timezone,
    required this.quiet_start,
    required this.quiet_end,
  });
  final String? token;
  final bool enabled;
  final String timezone;
  final int quiet_start;
  final int quiet_end;
  factory NotificationSettingsRequest.fromJson(Map<String, dynamic> json) =>
      NotificationSettingsRequest(
        token: json['token'] == null ? null : json['token'] as String,
        enabled: json['enabled'] as bool,
        timezone: json['timezone'] as String,
        quiet_start: json['quiet_start'] as int,
        quiet_end: json['quiet_end'] as int,
      );
  Map<String, dynamic> toJson() => {
    if (token != null) 'token': token,
    'enabled': enabled,
    'timezone': timezone,
    'quiet_start': quiet_start,
    'quiet_end': quiet_end,
  };
}

class ActionKeyRequest {
  const ActionKeyRequest({required this.public_key});
  final String public_key;
  factory ActionKeyRequest.fromJson(Map<String, dynamic> json) =>
      ActionKeyRequest(public_key: json['public_key'] as String);
  Map<String, dynamic> toJson() => {'public_key': public_key};
}

class ActionChallengeRequest {
  const ActionChallengeRequest({
    required this.job_id,
    required this.revision,
    required this.method,
  });
  final String job_id;
  final int revision;
  final String method;
  factory ActionChallengeRequest.fromJson(Map<String, dynamic> json) =>
      ActionChallengeRequest(
        job_id: json['job_id'] as String,
        revision: json['revision'] as int,
        method: json['method'] as String,
      );
  Map<String, dynamic> toJson() => {
    'job_id': job_id,
    'revision': revision,
    'method': method,
  };
}

class ActionChallengeResult {
  const ActionChallengeResult({
    required this.id,
    required this.challenge,
    required this.browser_url,
    required this.expires_at,
  });
  final String id;
  final String challenge;
  final String browser_url;
  final String expires_at;
  factory ActionChallengeResult.fromJson(Map<String, dynamic> json) =>
      ActionChallengeResult(
        id: json['id'] as String,
        challenge: json['challenge'] as String,
        browser_url: json['browser_url'] as String,
        expires_at: json['expires_at'] as String,
      );
  Map<String, dynamic> toJson() => {
    'id': id,
    'challenge': challenge,
    'browser_url': browser_url,
    'expires_at': expires_at,
  };
}

class ActionProofStatus {
  const ActionProofStatus({required this.approved});
  final bool approved;
  factory ActionProofStatus.fromJson(Map<String, dynamic> json) =>
      ActionProofStatus(approved: json['approved'] as bool);
  Map<String, dynamic> toJson() => {'approved': approved};
}

class ActionSignature {
  const ActionSignature({required this.signature});
  final String signature;
  factory ActionSignature.fromJson(Map<String, dynamic> json) =>
      ActionSignature(signature: json['signature'] as String);
  Map<String, dynamic> toJson() => {'signature': signature};
}

class ActionOptions {
  const ActionOptions({required this.options});
  final Map<String, dynamic> options;
  factory ActionOptions.fromJson(Map<String, dynamic> json) =>
      ActionOptions(options: Map<String, dynamic>.from(json['options'] as Map));
  Map<String, dynamic> toJson() => {'options': options};
}

class ActionCredential {
  const ActionCredential({required this.credential});
  final Map<String, dynamic> credential;
  factory ActionCredential.fromJson(Map<String, dynamic> json) =>
      ActionCredential(
        credential: Map<String, dynamic>.from(json['credential'] as Map),
      );
  Map<String, dynamic> toJson() => {'credential': credential};
}

class OperationIntent {
  const OperationIntent({
    required this.action,
    required this.target,
    required this.payload,
  });
  final String action;
  final String target;
  final Map<String, dynamic> payload;
  factory OperationIntent.fromJson(Map<String, dynamic> json) =>
      OperationIntent(
        action: json['action'] as String,
        target: json['target'] as String,
        payload: Map<String, dynamic>.from(json['payload'] as Map),
      );
  Map<String, dynamic> toJson() => {
    'action': action,
    'target': target,
    'payload': payload,
  };
}

class OperationsChallengeRequest {
  const OperationsChallengeRequest({
    required this.intent,
    required this.method,
  });
  final OperationIntent intent;
  final String method;
  factory OperationsChallengeRequest.fromJson(Map<String, dynamic> json) =>
      OperationsChallengeRequest(
        intent: OperationIntent.fromJson(
          json['intent'] as Map<String, dynamic>,
        ),
        method: json['method'] as String,
      );
  Map<String, dynamic> toJson() => {
    'intent': intent.toJson(),
    'method': method,
  };
}

class OperationsChallenge {
  const OperationsChallenge({
    required this.id,
    required this.challenge,
    required this.browser_url,
    required this.expires_at,
  });
  final String id;
  final String challenge;
  final String browser_url;
  final String expires_at;
  factory OperationsChallenge.fromJson(Map<String, dynamic> json) =>
      OperationsChallenge(
        id: json['id'] as String,
        challenge: json['challenge'] as String,
        browser_url: json['browser_url'] as String,
        expires_at: json['expires_at'] as String,
      );
  Map<String, dynamic> toJson() => {
    'id': id,
    'challenge': challenge,
    'browser_url': browser_url,
    'expires_at': expires_at,
  };
}

class OperationsExecuteRequest {
  const OperationsExecuteRequest({required this.intent, required this.proof});
  final OperationIntent intent;
  final String proof;
  factory OperationsExecuteRequest.fromJson(Map<String, dynamic> json) =>
      OperationsExecuteRequest(
        intent: OperationIntent.fromJson(
          json['intent'] as Map<String, dynamic>,
        ),
        proof: json['proof'] as String,
      );
  Map<String, dynamic> toJson() => {'intent': intent.toJson(), 'proof': proof};
}

class OperationsSignatureRequest {
  const OperationsSignatureRequest({required this.signature});
  final String signature;
  factory OperationsSignatureRequest.fromJson(Map<String, dynamic> json) =>
      OperationsSignatureRequest(signature: json['signature'] as String);
  Map<String, dynamic> toJson() => {'signature': signature};
}

class OperationsCredentialRequest {
  const OperationsCredentialRequest({required this.credential});
  final Map<String, dynamic> credential;
  factory OperationsCredentialRequest.fromJson(Map<String, dynamic> json) =>
      OperationsCredentialRequest(
        credential: Map<String, dynamic>.from(json['credential'] as Map),
      );
  Map<String, dynamic> toJson() => {'credential': credential};
}

class OperationsProofStatus {
  const OperationsProofStatus({required this.approved});
  final bool approved;
  factory OperationsProofStatus.fromJson(Map<String, dynamic> json) =>
      OperationsProofStatus(approved: json['approved'] as bool);
  Map<String, dynamic> toJson() => {'approved': approved};
}

class OperationsProofOptions {
  const OperationsProofOptions({
    required this.options,
    required this.action,
    required this.target,
  });
  final Map<String, dynamic> options;
  final String action;
  final String target;
  factory OperationsProofOptions.fromJson(Map<String, dynamic> json) =>
      OperationsProofOptions(
        options: Map<String, dynamic>.from(json['options'] as Map),
        action: json['action'] as String,
        target: json['target'] as String,
      );
  Map<String, dynamic> toJson() => {
    'options': options,
    'action': action,
    'target': target,
  };
}

class OperationsOverview {
  const OperationsOverview({
    required this.origin,
    required this.observed_at,
    required this.targets,
    required this.sessions,
    required this.secrets,
    required this.vault,
  });
  final String origin;
  final String observed_at;
  final List<Map<String, dynamic>> targets;
  final List<Map<String, dynamic>> sessions;
  final List<Map<String, dynamic>> secrets;
  final bool vault;
  factory OperationsOverview.fromJson(Map<String, dynamic> json) =>
      OperationsOverview(
        origin: json['origin'] as String,
        observed_at: json['observed_at'] as String,
        targets: (json['targets'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
        sessions: (json['sessions'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
        secrets: (json['secrets'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
        vault: json['vault'] as bool,
      );
  Map<String, dynamic> toJson() => {
    'origin': origin,
    'observed_at': observed_at,
    'targets': targets,
    'sessions': sessions,
    'secrets': secrets,
    'vault': vault,
  };
}

class OperationsResult {
  const OperationsResult(this.data);
  final Map<String, dynamic> data;
  factory OperationsResult.fromJson(Map<String, dynamic> json) =>
      OperationsResult(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class RecoveryImportRequest {
  const RecoveryImportRequest({required this.batch, required this.signature});
  final Map<String, dynamic> batch;
  final String signature;
  factory RecoveryImportRequest.fromJson(Map<String, dynamic> json) =>
      RecoveryImportRequest(
        batch: Map<String, dynamic>.from(json['batch'] as Map),
        signature: json['signature'] as String,
      );
  Map<String, dynamic> toJson() => {'batch': batch, 'signature': signature};
}

class RecoveryImportResult {
  const RecoveryImportResult({required this.cursor});
  final int cursor;
  factory RecoveryImportResult.fromJson(Map<String, dynamic> json) =>
      RecoveryImportResult(cursor: json['cursor'] as int);
  Map<String, dynamic> toJson() => {'cursor': cursor};
}

class BackupRequest {
  const BackupRequest({
    required this.policy,
    required this.kind,
    required this.artifact,
    required this.target,
  });
  final String policy;
  final String kind;
  final String artifact;
  final String target;
  factory BackupRequest.fromJson(Map<String, dynamic> json) => BackupRequest(
    policy: json['policy'] as String,
    kind: json['kind'] as String,
    artifact: json['artifact'] as String,
    target: json['target'] as String,
  );
  Map<String, dynamic> toJson() => {
    'policy': policy,
    'kind': kind,
    'artifact': artifact,
    'target': target,
  };
}

class BackupDecision {
  const BackupDecision({
    required this.action,
    required this.revision,
    required this.phrase,
    required this.proof,
  });
  final String action;
  final int revision;
  final String phrase;
  final String proof;
  factory BackupDecision.fromJson(Map<String, dynamic> json) => BackupDecision(
    action: json['action'] as String,
    revision: json['revision'] as int,
    phrase: json['phrase'] as String,
    proof: json['proof'] as String,
  );
  Map<String, dynamic> toJson() => {
    'action': action,
    'revision': revision,
    'phrase': phrase,
    'proof': proof,
  };
}

class BackupJob {
  const BackupJob({
    required this.id,
    required this.policy_id,
    required this.kind,
    required this.artifact_id,
    required this.target_id,
    required this.state,
    required this.step,
    required this.revision,
    required this.scheduled,
    required this.created_at,
    required this.expires_at,
    required this.started_at,
    required this.finished_at,
    required this.error_code,
    required this.evidence,
  });
  final String id;
  final String policy_id;
  final String kind;
  final String artifact_id;
  final String target_id;
  final String state;
  final String step;
  final int revision;
  final bool scheduled;
  final String created_at;
  final String expires_at;
  final String? started_at;
  final String? finished_at;
  final String error_code;
  final Map<String, dynamic> evidence;
  factory BackupJob.fromJson(Map<String, dynamic> json) => BackupJob(
    id: json['id'] as String,
    policy_id: json['policy_id'] as String,
    kind: json['kind'] as String,
    artifact_id: json['artifact_id'] as String,
    target_id: json['target_id'] as String,
    state: json['state'] as String,
    step: json['step'] as String,
    revision: json['revision'] as int,
    scheduled: json['scheduled'] as bool,
    created_at: json['created_at'] as String,
    expires_at: json['expires_at'] as String,
    started_at: json['started_at'] == null
        ? null
        : json['started_at'] as String,
    finished_at: json['finished_at'] == null
        ? null
        : json['finished_at'] as String,
    error_code: json['error_code'] as String,
    evidence: Map<String, dynamic>.from(json['evidence'] as Map),
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'policy_id': policy_id,
    'kind': kind,
    'artifact_id': artifact_id,
    'target_id': target_id,
    'state': state,
    'step': step,
    'revision': revision,
    'scheduled': scheduled,
    'created_at': created_at,
    'expires_at': expires_at,
    'started_at': started_at,
    'finished_at': finished_at,
    'error_code': error_code,
    'evidence': evidence,
  };
}

class BackupArtifact {
  const BackupArtifact({
    required this.id,
    required this.policy_id,
    required this.job_id,
    required this.snapshot_id,
    required this.bytes,
    required this.created_at,
    required this.verified_at,
    required this.expired_at,
  });
  final String id;
  final String policy_id;
  final String job_id;
  final String snapshot_id;
  final int bytes;
  final String created_at;
  final String verified_at;
  final String? expired_at;
  factory BackupArtifact.fromJson(Map<String, dynamic> json) => BackupArtifact(
    id: json['id'] as String,
    policy_id: json['policy_id'] as String,
    job_id: json['job_id'] as String,
    snapshot_id: json['snapshot_id'] as String,
    bytes: json['bytes'] as int,
    created_at: json['created_at'] as String,
    verified_at: json['verified_at'] as String,
    expired_at: json['expired_at'] == null
        ? null
        : json['expired_at'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'policy_id': policy_id,
    'job_id': job_id,
    'snapshot_id': snapshot_id,
    'bytes': bytes,
    'created_at': created_at,
    'verified_at': verified_at,
    'expired_at': expired_at,
  };
}

class BackupOverview {
  const BackupOverview({
    required this.enabled,
    required this.policies,
    required this.targets,
    required this.jobs,
    required this.artifacts,
    required this.has_more,
  });
  final bool enabled;
  final List<Map<String, dynamic>> policies;
  final List<Map<String, dynamic>> targets;
  final List<BackupJob> jobs;
  final List<BackupArtifact> artifacts;
  final bool has_more;
  factory BackupOverview.fromJson(Map<String, dynamic> json) => BackupOverview(
    enabled: json['enabled'] as bool,
    policies: (json['policies'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
    targets: (json['targets'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
    jobs: (json['jobs'] as List)
        .map((v) => BackupJob.fromJson(v as Map<String, dynamic>))
        .toList(),
    artifacts: (json['artifacts'] as List)
        .map((v) => BackupArtifact.fromJson(v as Map<String, dynamic>))
        .toList(),
    has_more: json['has_more'] as bool,
  );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'policies': policies,
    'targets': targets,
    'jobs': jobs.map((v) => v.toJson()).toList(),
    'artifacts': artifacts.map((v) => v.toJson()).toList(),
    'has_more': has_more,
  };
}

class BackupJobResponse {
  const BackupJobResponse({required this.job, required this.confirmation});
  final BackupJob job;
  final String confirmation;
  factory BackupJobResponse.fromJson(Map<String, dynamic> json) =>
      BackupJobResponse(
        job: BackupJob.fromJson(json['job'] as Map<String, dynamic>),
        confirmation: json['confirmation'] as String,
      );
  Map<String, dynamic> toJson() => {
    'job': job.toJson(),
    'confirmation': confirmation,
  };
}

class BackupDetail {
  const BackupDetail({
    required this.job,
    required this.confirmation,
    required this.events,
    required this.plan,
  });
  final BackupJob job;
  final String confirmation;
  final List<Map<String, dynamic>> events;
  final Map<String, dynamic> plan;
  factory BackupDetail.fromJson(Map<String, dynamic> json) => BackupDetail(
    job: BackupJob.fromJson(json['job'] as Map<String, dynamic>),
    confirmation: json['confirmation'] as String,
    events: (json['events'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
    plan: Map<String, dynamic>.from(json['plan'] as Map),
  );
  Map<String, dynamic> toJson() => {
    'job': job.toJson(),
    'confirmation': confirmation,
    'events': events,
    'plan': plan,
  };
}

class EdgeRequest {
  const EdgeRequest({
    required this.target,
    required this.value,
    required this.rollback,
  });
  final String target;
  final String value;
  final String rollback;
  factory EdgeRequest.fromJson(Map<String, dynamic> json) => EdgeRequest(
    target: json['target'] as String,
    value: json['value'] as String,
    rollback: json['rollback'] as String,
  );
  Map<String, dynamic> toJson() => {
    'target': target,
    'value': value,
    'rollback': rollback,
  };
}

class EdgeDecision {
  const EdgeDecision({
    required this.action,
    required this.revision,
    required this.phrase,
    required this.proof,
  });
  final String action;
  final int revision;
  final String phrase;
  final String proof;
  factory EdgeDecision.fromJson(Map<String, dynamic> json) => EdgeDecision(
    action: json['action'] as String,
    revision: json['revision'] as int,
    phrase: json['phrase'] as String,
    proof: json['proof'] as String,
  );
  Map<String, dynamic> toJson() => {
    'action': action,
    'revision': revision,
    'phrase': phrase,
    'proof': proof,
  };
}

class EdgePlan {
  const EdgePlan({
    required this.before,
    required this.after,
    required this.impact,
    required this.verification,
    required this.rollback,
    required this.checks,
  });
  final String before;
  final String after;
  final String impact;
  final String verification;
  final String rollback;
  final List<Map<String, dynamic>> checks;
  factory EdgePlan.fromJson(Map<String, dynamic> json) => EdgePlan(
    before: json['before'] as String,
    after: json['after'] as String,
    impact: json['impact'] as String,
    verification: json['verification'] as String,
    rollback: json['rollback'] as String,
    checks: (json['checks'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
  );
  Map<String, dynamic> toJson() => {
    'before': before,
    'after': after,
    'impact': impact,
    'verification': verification,
    'rollback': rollback,
    'checks': checks,
  };
}

class EdgeJob {
  const EdgeJob({
    required this.id,
    required this.target,
    required this.request,
    required this.state,
    required this.revision,
    required this.created_at,
    required this.expires_at,
    required this.plan,
    required this.result,
    required this.error_code,
  });
  final String id;
  final String target;
  final EdgeRequest request;
  final String state;
  final int revision;
  final String created_at;
  final String expires_at;
  final EdgePlan plan;
  final Map<String, dynamic> result;
  final String error_code;
  factory EdgeJob.fromJson(Map<String, dynamic> json) => EdgeJob(
    id: json['id'] as String,
    target: json['target'] as String,
    request: EdgeRequest.fromJson(json['request'] as Map<String, dynamic>),
    state: json['state'] as String,
    revision: json['revision'] as int,
    created_at: json['created_at'] as String,
    expires_at: json['expires_at'] as String,
    plan: EdgePlan.fromJson(json['plan'] as Map<String, dynamic>),
    result: Map<String, dynamic>.from(json['result'] as Map),
    error_code: json['error_code'] as String,
  );
  Map<String, dynamic> toJson() => {
    'id': id,
    'target': target,
    'request': request.toJson(),
    'state': state,
    'revision': revision,
    'created_at': created_at,
    'expires_at': expires_at,
    'plan': plan.toJson(),
    'result': result,
    'error_code': error_code,
  };
}

class EdgeJobResponse {
  const EdgeJobResponse({required this.job, required this.confirmation});
  final EdgeJob job;
  final String confirmation;
  factory EdgeJobResponse.fromJson(Map<String, dynamic> json) =>
      EdgeJobResponse(
        job: EdgeJob.fromJson(json['job'] as Map<String, dynamic>),
        confirmation: json['confirmation'] as String,
      );
  Map<String, dynamic> toJson() => {
    'job': job.toJson(),
    'confirmation': confirmation,
  };
}

class EdgeOverview {
  const EdgeOverview({
    required this.enabled,
    required this.targets,
    required this.observations,
    required this.jobs,
    required this.has_more,
  });
  final bool enabled;
  final List<Map<String, dynamic>> targets;
  final List<Map<String, dynamic>> observations;
  final List<EdgeJob> jobs;
  final bool has_more;
  factory EdgeOverview.fromJson(Map<String, dynamic> json) => EdgeOverview(
    enabled: json['enabled'] as bool,
    targets: (json['targets'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
    observations: (json['observations'] as List)
        .map((v) => Map<String, dynamic>.from(v as Map))
        .toList(),
    jobs: (json['jobs'] as List)
        .map((v) => EdgeJob.fromJson(v as Map<String, dynamic>))
        .toList(),
    has_more: json['has_more'] as bool,
  );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'targets': targets,
    'observations': observations,
    'jobs': jobs.map((v) => v.toJson()).toList(),
    'has_more': has_more,
  };
}

class AutomationControl {
  const AutomationControl(this.data);
  final Map<String, dynamic> data;
  factory AutomationControl.fromJson(Map<String, dynamic> json) =>
      AutomationControl(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationStep {
  const AutomationStep(this.data);
  final Map<String, dynamic> data;
  factory AutomationStep.fromJson(Map<String, dynamic> json) =>
      AutomationStep(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationParameter {
  const AutomationParameter(this.data);
  final Map<String, dynamic> data;
  factory AutomationParameter.fromJson(Map<String, dynamic> json) =>
      AutomationParameter(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationRunbook {
  const AutomationRunbook(this.data);
  final Map<String, dynamic> data;
  factory AutomationRunbook.fromJson(Map<String, dynamic> json) =>
      AutomationRunbook(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationPolicy {
  const AutomationPolicy(this.data);
  final Map<String, dynamic> data;
  factory AutomationPolicy.fromJson(Map<String, dynamic> json) =>
      AutomationPolicy(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationRunRequest {
  const AutomationRunRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationRunRequest.fromJson(Map<String, dynamic> json) =>
      AutomationRunRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationSchedule {
  const AutomationSchedule(this.data);
  final Map<String, dynamic> data;
  factory AutomationSchedule.fromJson(Map<String, dynamic> json) =>
      AutomationSchedule(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationOverview {
  const AutomationOverview(this.data);
  final Map<String, dynamic> data;
  factory AutomationOverview.fromJson(Map<String, dynamic> json) =>
      AutomationOverview(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationDefinitionRequest {
  const AutomationDefinitionRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationDefinitionRequest.fromJson(Map<String, dynamic> json) =>
      AutomationDefinitionRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationControlRequest {
  const AutomationControlRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationControlRequest.fromJson(Map<String, dynamic> json) =>
      AutomationControlRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationScheduleRequest {
  const AutomationScheduleRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationScheduleRequest.fromJson(Map<String, dynamic> json) =>
      AutomationScheduleRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationDecisionRequest {
  const AutomationDecisionRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationDecisionRequest.fromJson(Map<String, dynamic> json) =>
      AutomationDecisionRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationResetRequest {
  const AutomationResetRequest(this.data);
  final Map<String, dynamic> data;
  factory AutomationResetRequest.fromJson(Map<String, dynamic> json) =>
      AutomationResetRequest(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class AutomationResult {
  const AutomationResult(this.data);
  final Map<String, dynamic> data;
  factory AutomationResult.fromJson(Map<String, dynamic> json) =>
      AutomationResult(Map<String, dynamic>.from(json));
  Map<String, dynamic> toJson() => data;
}

class OnboardingOverview {
  const OnboardingOverview({
    required this.enabled,
    required this.targets,
    required this.projects,
  });
  final bool enabled;
  final List<Map<String, dynamic>> targets;
  final List<Map<String, dynamic>> projects;
  factory OnboardingOverview.fromJson(Map<String, dynamic> json) =>
      OnboardingOverview(
        enabled: json['enabled'] as bool,
        targets: (json['targets'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
        projects: (json['projects'] as List)
            .map((v) => Map<String, dynamic>.from(v as Map))
            .toList(),
      );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'targets': targets,
    'projects': projects,
  };
}

class OnboardingRequest {
  const OnboardingRequest({required this.target, required this.release});
  final String target;
  final String release;
  factory OnboardingRequest.fromJson(Map<String, dynamic> json) =>
      OnboardingRequest(
        target: json['target'] as String,
        release: json['release'] as String,
      );
  Map<String, dynamic> toJson() => {'target': target, 'release': release};
}

class OnboardingPrepared {
  const OnboardingPrepared({required this.job, required this.files});
  final DeploymentJob job;
  final Map<String, String> files;
  factory OnboardingPrepared.fromJson(Map<String, dynamic> json) =>
      OnboardingPrepared(
        job: DeploymentJob.fromJson(json['job'] as Map<String, dynamic>),
        files: Map<String, String>.from(json['files'] as Map),
      );
  Map<String, dynamic> toJson() => {'job': job.toJson(), 'files': files};
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
    Capabilities request, {
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

  Future<ServerPage> servers({String cursor = ''}) async {
    final json = await send(
      'GET',
      '/v1/servers?cursor=${Uri.encodeQueryComponent(cursor)}',
    );
    return ServerPage.fromJson(json);
  }

  Future<ServerRecord> serverDetail({required String id}) async {
    final json = await send('GET', '/v1/servers/${Uri.encodeComponent(id)}');
    return ServerRecord.fromJson(json);
  }

  Future<void> revokeServer({required String id}) async {
    final json = await send('DELETE', '/v1/servers/${Uri.encodeComponent(id)}');
  }

  Future<Capabilities> setCapabilities(
    Capabilities request, {
    required String id,
  }) async {
    final json = await send(
      'PUT',
      '/v1/servers/${Uri.encodeComponent(id)}/capabilities',
      body: request.toJson(),
    );
    return Capabilities.fromJson(json);
  }

  Future<DevicePage> devices() async {
    final json = await send('GET', '/v1/devices');
    return DevicePage.fromJson(json);
  }

  Future<void> revokeDevice({required String id}) async {
    final json = await send('DELETE', '/v1/devices/${Uri.encodeComponent(id)}');
  }

  Future<DevicePreferences> preferences() async {
    final json = await send('GET', '/v1/preferences');
    return DevicePreferences.fromJson(json);
  }

  Future<DevicePreferences> savePreferences(DevicePreferences request) async {
    final json = await send('PUT', '/v1/preferences', body: request.toJson());
    return DevicePreferences.fromJson(json);
  }

  Future<AuditPage> audit({String cursor = ''}) async {
    final json = await send(
      'GET',
      '/v1/audit?cursor=${Uri.encodeQueryComponent(cursor)}',
    );
    return AuditPage.fromJson(json);
  }

  Future<RecoveryCodes> recoveryCodes() async {
    final json = await send('POST', '/v1/recovery-codes');
    return RecoveryCodes.fromJson(json);
  }

  Future<RegistrationTicket> registrationTicket() async {
    final json = await send('POST', '/v1/registration-ticket');
    return RegistrationTicket.fromJson(json);
  }

  Future<Session> rotateSession() async {
    final json = await send('POST', '/v1/session/rotate');
    return Session.fromJson(json);
  }

  Future<Enrollment> renewAgent(RenewalRequest request) async {
    final json = await send('POST', '/v1/agent/renew', body: request.toJson());
    return Enrollment.fromJson(json);
  }

  Future<Accepted> publishInventory(InventorySnapshot request) async {
    final json = await send(
      'POST',
      '/v1/agent/inventory',
      body: request.toJson(),
    );
    return Accepted.fromJson(json);
  }

  Future<ImportResult> importInventory(ImportRequest request) async {
    final json = await send(
      'POST',
      '/v1/inventory-imports',
      body: request.toJson(),
    );
    return ImportResult.fromJson(json);
  }

  Future<BrowserLogin> beginBrowser(BrowserLoginRequest request) async {
    final json = await send('POST', '/v1/auth/browser', body: request.toJson());
    return BrowserLogin.fromJson(json);
  }

  Future<PollResult> pollBrowser(PollRequest request) async {
    final json = await send('POST', '/v1/auth/poll', body: request.toJson());
    return PollResult.fromJson(json);
  }

  Future<PasskeyBegin> beginPasskey(PasskeyBeginRequest request) async {
    final json = await send('POST', '/v1/auth/begin', body: request.toJson());
    return PasskeyBegin.fromJson(json);
  }

  Future<Accepted> finishPasskey(PasskeyFinish request) async {
    final json = await send('POST', '/v1/auth/finish', body: request.toJson());
    return Accepted.fromJson(json);
  }

  Future<ObservationOverview> observationOverview() async {
    final json = await send('GET', '/v1/observability');
    return ObservationOverview.fromJson(json);
  }

  Future<MetricResult> observationMetrics({
    required String scope,
    required String metric,
    required String start,
    required String end,
  }) async {
    final json = await send(
      'GET',
      '/v1/observability/metrics?scope=${Uri.encodeQueryComponent(scope)}&metric=${Uri.encodeQueryComponent(metric)}&start=${Uri.encodeQueryComponent(start)}&end=${Uri.encodeQueryComponent(end)}',
    );
    return MetricResult.fromJson(json);
  }

  Future<LogResult> observationLogs({
    required String scope,
    required String start,
    required String end,
    String search = '',
    String level = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/observability/logs?scope=${Uri.encodeQueryComponent(scope)}&start=${Uri.encodeQueryComponent(start)}&end=${Uri.encodeQueryComponent(end)}&search=${Uri.encodeQueryComponent(search)}&level=${Uri.encodeQueryComponent(level)}',
    );
    return LogResult.fromJson(json);
  }

  Future<IncidentPage> incidents({
    String cursor = '',
    String scope = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/incidents?cursor=${Uri.encodeQueryComponent(cursor)}&scope=${Uri.encodeQueryComponent(scope)}',
    );
    return IncidentPage.fromJson(json);
  }

  Future<IncidentDetail> incidentDetail({
    required String id,
    String cursor = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/incidents/${Uri.encodeComponent(id)}?cursor=${Uri.encodeQueryComponent(cursor)}',
    );
    return IncidentDetail.fromJson(json);
  }

  Future<IncidentActionResult> incidentAction(
    IncidentAction request, {
    required String id,
  }) async {
    final json = await send(
      'POST',
      '/v1/incidents/${Uri.encodeComponent(id)}/events',
      body: request.toJson(),
    );
    return IncidentActionResult.fromJson(json);
  }

  Future<GithubOverview> githubOverview() async {
    final json = await send('GET', '/v1/github');
    return GithubOverview.fromJson(json);
  }

  Future<GithubBuildPage> githubBuilds({
    required String repository,
    String page = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/github/repositories/${Uri.encodeComponent(repository)}/builds?page=${Uri.encodeQueryComponent(page)}',
    );
    return GithubBuildPage.fromJson(json);
  }

  Future<GithubReleasePage> githubReleases({
    required String repository,
    String page = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/github/repositories/${Uri.encodeComponent(repository)}/releases?page=${Uri.encodeQueryComponent(page)}',
    );
    return GithubReleasePage.fromJson(json);
  }

  Future<GithubData> githubBrowse({
    required String repository,
    String kind = '',
    String ref = '',
    String path = '',
    String page = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/github/repositories/${Uri.encodeComponent(repository)}/browse?kind=${Uri.encodeQueryComponent(kind)}&ref=${Uri.encodeQueryComponent(ref)}&path=${Uri.encodeQueryComponent(path)}&page=${Uri.encodeQueryComponent(page)}',
    );
    return GithubData.fromJson(json);
  }

  Future<GithubData> githubBuild({
    required String repository,
    required String run,
    String page = '',
  }) async {
    final json = await send(
      'GET',
      '/v1/github/repositories/${Uri.encodeComponent(repository)}/builds/${Uri.encodeComponent(run)}?page=${Uri.encodeQueryComponent(page)}',
    );
    return GithubData.fromJson(json);
  }

  Future<GithubLog> githubLogs({
    required String repository,
    required String run,
    required String job,
  }) async {
    final json = await send(
      'GET',
      '/v1/github/repositories/${Uri.encodeComponent(repository)}/builds/${Uri.encodeComponent(run)}/jobs/${Uri.encodeComponent(job)}/logs',
    );
    return GithubLog.fromJson(json);
  }

  Future<DeploymentOverview> deployments({String page = ''}) async {
    final json = await send(
      'GET',
      '/v1/deployments?page=${Uri.encodeQueryComponent(page)}',
    );
    return DeploymentOverview.fromJson(json);
  }

  Future<DeploymentPrepared> prepareDeployment(
    DeploymentRequest request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/deployments',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return DeploymentPrepared.fromJson(json);
  }

  Future<DeploymentDetail> deployment({required String job}) async {
    final json = await send(
      'GET',
      '/v1/deployments/${Uri.encodeComponent(job)}',
    );
    return DeploymentDetail.fromJson(json);
  }

  Future<DeploymentPrepared> decideDeployment(
    DeploymentDecision request, {
    required String job,
  }) async {
    final json = await send(
      'POST',
      '/v1/deployments/${Uri.encodeComponent(job)}/decision',
      body: request.toJson(),
    );
    return DeploymentPrepared.fromJson(json);
  }

  Future<NotificationSettings> notificationSettings() async {
    final json = await send('GET', '/v1/notifications');
    return NotificationSettings.fromJson(json);
  }

  Future<Status> saveNotificationSettings(
    NotificationSettingsRequest request,
  ) async {
    final json = await send('PUT', '/v1/notifications', body: request.toJson());
    return Status.fromJson(json);
  }

  Future<Status> disableNotifications() async {
    final json = await send('DELETE', '/v1/notifications');
    return Status.fromJson(json);
  }

  Future<IncidentDetail> notificationIncident({required String public}) async {
    final json = await send(
      'GET',
      '/v1/notifications/incidents/${Uri.encodeComponent(public)}',
    );
    return IncidentDetail.fromJson(json);
  }

  Future<Status> enrollActionKey(ActionKeyRequest request) async {
    final json = await send('POST', '/v1/action-key', body: request.toJson());
    return Status.fromJson(json);
  }

  Future<ActionChallengeResult> beginActionChallenge(
    ActionChallengeRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/action-challenges',
      body: request.toJson(),
    );
    return ActionChallengeResult.fromJson(json);
  }

  Future<ActionProofStatus> actionProofStatus({
    required String challenge,
  }) async {
    final json = await send(
      'GET',
      '/v1/action-challenges/${Uri.encodeComponent(challenge)}',
    );
    return ActionProofStatus.fromJson(json);
  }

  Future<ActionProofStatus> signActionProof(
    ActionSignature request, {
    required String challenge,
  }) async {
    final json = await send(
      'POST',
      '/v1/action-challenges/${Uri.encodeComponent(challenge)}/sign',
      body: request.toJson(),
    );
    return ActionProofStatus.fromJson(json);
  }

  Future<ActionOptions> actionPasskeyOptions({
    required String challenge,
  }) async {
    final json = await send(
      'GET',
      '/v1/action-challenges/${Uri.encodeComponent(challenge)}/options',
    );
    return ActionOptions.fromJson(json);
  }

  Future<ActionProofStatus> verifyActionPasskey(
    ActionCredential request, {
    required String challenge,
  }) async {
    final json = await send(
      'POST',
      '/v1/action-challenges/${Uri.encodeComponent(challenge)}/passkey',
      body: request.toJson(),
    );
    return ActionProofStatus.fromJson(json);
  }

  Future<OperationsOverview> operationsOverview() async {
    final json = await send('GET', '/v1/operations/overview');
    return OperationsOverview.fromJson(json);
  }

  Future<OperationsChallenge> operationsChallenge(
    OperationsChallengeRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/operations/challenges',
      body: request.toJson(),
    );
    return OperationsChallenge.fromJson(json);
  }

  Future<OperationsProofStatus> operationsProofStatus({
    required String id,
  }) async {
    final json = await send(
      'GET',
      '/v1/operations/challenges/${Uri.encodeComponent(id)}',
    );
    return OperationsProofStatus.fromJson(json);
  }

  Future<OperationsProofStatus> operationsSign(
    OperationsSignatureRequest request, {
    required String id,
  }) async {
    final json = await send(
      'POST',
      '/v1/operations/challenges/${Uri.encodeComponent(id)}/sign',
      body: request.toJson(),
    );
    return OperationsProofStatus.fromJson(json);
  }

  Future<OperationsResult> operationsExecute(
    OperationsExecuteRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/operations/execute',
      body: request.toJson(),
    );
    return OperationsResult.fromJson(json);
  }

  Future<RecoveryImportResult> importRecoveryAudit(
    RecoveryImportRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/recovery/import',
      body: request.toJson(),
    );
    return RecoveryImportResult.fromJson(json);
  }

  Future<BackupOverview> backups({String page = ''}) async {
    final json = await send(
      'GET',
      '/v1/backups?page=${Uri.encodeQueryComponent(page)}',
    );
    return BackupOverview.fromJson(json);
  }

  Future<BackupJobResponse> prepareBackup(
    BackupRequest request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/backups/jobs',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return BackupJobResponse.fromJson(json);
  }

  Future<BackupDetail> backupJob({required String job}) async {
    final json = await send(
      'GET',
      '/v1/backups/jobs/${Uri.encodeComponent(job)}',
    );
    return BackupDetail.fromJson(json);
  }

  Future<BackupJobResponse> decideBackup(
    BackupDecision request, {
    required String job,
  }) async {
    final json = await send(
      'POST',
      '/v1/backups/jobs/${Uri.encodeComponent(job)}/decision',
      body: request.toJson(),
    );
    return BackupJobResponse.fromJson(json);
  }

  Future<EdgeOverview> edge({String page = ''}) async {
    final json = await send(
      'GET',
      '/v1/edge?page=${Uri.encodeQueryComponent(page)}',
    );
    return EdgeOverview.fromJson(json);
  }

  Future<EdgeJobResponse> prepareEdge(
    EdgeRequest request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/edge/jobs',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return EdgeJobResponse.fromJson(json);
  }

  Future<EdgeJobResponse> edgeJob({required String job}) async {
    final json = await send('GET', '/v1/edge/jobs/${Uri.encodeComponent(job)}');
    return EdgeJobResponse.fromJson(json);
  }

  Future<EdgeJobResponse> decideEdge(
    EdgeDecision request, {
    required String job,
  }) async {
    final json = await send(
      'POST',
      '/v1/edge/jobs/${Uri.encodeComponent(job)}/decision',
      body: request.toJson(),
    );
    return EdgeJobResponse.fromJson(json);
  }

  Future<AutomationOverview> automation({String page = ''}) async {
    final json = await send(
      'GET',
      '/v1/automation?page=${Uri.encodeQueryComponent(page)}',
    );
    return AutomationOverview.fromJson(json);
  }

  Future<AutomationResult> publishAutomation(
    AutomationDefinitionRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/automation/definitions',
      body: request.toJson(),
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> controlAutomation(
    AutomationControlRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/automation/control',
      body: request.toJson(),
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> prepareAutomation(
    AutomationRunRequest request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/automation/runs',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> automationRun({required String run}) async {
    final json = await send(
      'GET',
      '/v1/automation/runs/${Uri.encodeComponent(run)}',
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> decideAutomation(
    AutomationDecisionRequest request, {
    required String run,
  }) async {
    final json = await send(
      'POST',
      '/v1/automation/runs/${Uri.encodeComponent(run)}/decision',
      body: request.toJson(),
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> scheduleAutomation(
    AutomationScheduleRequest request,
  ) async {
    final json = await send(
      'POST',
      '/v1/automation/schedules',
      body: request.toJson(),
    );
    return AutomationResult.fromJson(json);
  }

  Future<AutomationResult> resetAutomation(
    AutomationResetRequest request, {
    required String policy,
  }) async {
    final json = await send(
      'POST',
      '/v1/automation/policies/${Uri.encodeComponent(policy)}/reset',
      body: request.toJson(),
    );
    return AutomationResult.fromJson(json);
  }

  Future<OnboardingOverview> onboarding() async {
    final json = await send('GET', '/v1/onboarding');
    return OnboardingOverview.fromJson(json);
  }

  Future<OnboardingPrepared> onboardProject(
    OnboardingRequest request, {
    required String idempotencyKey,
  }) async {
    final json = await send(
      'POST',
      '/v1/onboarding',
      body: request.toJson(),
      idempotencyKey: idempotencyKey,
    );
    return OnboardingPrepared.fromJson(json);
  }
}
