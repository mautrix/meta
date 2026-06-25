namespace go requeststream

enum FlowStatus {
  Accepted = 1,
  Started = 2,
  Stopped = 3,
}

enum TerminationReason {
  Rejected = 40,
  Error = 50,
  TryAgain = 80,
  Closed = 99,
}

enum AckLevel {
  BestEffort = 0,
  Socket = 10,
  Device = 20,
}

enum Ack {
  Ignored = 100,
  Success = 200,
  Queued = 250,
  UnknownStream = 400,
  HandledUnknownResult = 450,
  Failure = 500,
}

struct ResponseRewriteRequest {
  1: optional string new_headers,
  2: optional binary new_body,
  3: optional string patch_headers,
}

struct ResponseData {
  1: required binary bytes,
  2: optional string metadata,
}

struct LogMessage {
  1: required string message,
}

struct Termination {
  1: required TerminationReason reason,
  2: optional string message,
  3: optional i64 retry_delay_ms,
}

struct AmendStreamAck {
  1: optional i64 amendment_id,
  2: optional bool accepted,
  3: optional binary result,
}

struct StreamCheck {
  1: required i64 stream_id,
  2: required i32 last_sequencer,
  3: required FlowStatus last_status,
  4: required i32 amendment_count,
}

struct Ping {
  1: required i64 ping_id,
  2: required i64 caller_timestamp_ms,
}

struct Pong {
  1: required i64 ping_id,
  2: required i64 original_ping_timestamp_ms,
}

union StreamResponseDelta {
  1: FlowStatus flow_status,
  2: LogMessage log,
  3: ResponseRewriteRequest rewrite,
  4: ResponseData data,
  5: Termination termination,
  6: AmendStreamAck amend_ack,
}

struct RequestStreamBody {
  1: required binary body,
}

struct AmendStream {
  1: optional i64 amendment_id,
  2: required binary amendment,
  4: optional binary instrumentation_data,
}

struct StreamResponseAck {
  1: optional i64 response_id,
  2: required Ack ack,
}

struct StreamResponse {
  1: optional i64 response_id,
  2: required list<StreamResponseDelta> delta,
  3: optional AckLevel ack_level,
  4: optional binary query_request,
  5: optional binary instrumentation_data,
}

union Payload {
  2: RequestStreamBody request_body,
  3: AmendStream amend,
  4: StreamResponseAck ack,
  5: StreamResponse response,
  6: Ping ping,
  7: Pong pong,
}
