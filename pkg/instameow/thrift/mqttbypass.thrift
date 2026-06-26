namespace go mqttbypass

enum ErrorCode {
  Unknown = 0,
  Timeout = 1,
}

struct ClientTraceInfo {
  1: required i64 traceId,
  2: required list<string> samplingReasons,
}

struct PublishRequest {
  1: required binary payload,
  2: required string mqttTopic,
  3: optional ClientTraceInfo clientTraceInfo,
}

struct PublishResponse {
  1: required binary payload,
  2: required string mqttTopic,
}

struct SubscribeRequest {
  1: required list<string> topics,
}

struct UnsubscribeRequest {
  1: required list<string> topics,
}

struct ConnectRequest {
  1: required i64 sessionId,
  2: required i64 clientCapabilities,
  3: required string userAgent,
  4: required string deviceId,
  5: required string familyDeviceId,
  6: required SubscribeRequest subscription,
  7: optional string appSpecificInfo,
  8: optional list<PublishRequest> publishes,
}

struct CreateIrisQueueRequest {
  1: optional i64 initial_titan_sequence_id,
  2: optional string queue_type,
  3: optional string queue_position,
  4: optional i32 sync_api_version,
  5: optional i32 max_deltas_able_to_process,
  6: optional i32 delta_batch_size,
  7: optional string encoding,
  8: optional i64 entity_fbid,
  9: optional string sync_device_id,
  10: optional string client_feature_id,
}

struct ErrorResponse {
  1: required ErrorCode code,
  2: required string message,
}

union RequestPayload {
  1: ConnectRequest connectRequest,
  2: PublishRequest publishRequest,
  3: SubscribeRequest subscribeRequest,
  4: UnsubscribeRequest unsubscribeRequest,
}

union ResponsePayload {
  1: PublishResponse publishResponse,
  2: ErrorResponse errorResponse,
}
