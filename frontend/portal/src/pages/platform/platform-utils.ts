import { useAuth } from '@/contexts/AuthContext';
import { ApiRequestError } from '@/services/queries/rules';
import type { EventPropertySchema } from '@/services/api/models/events';

/** True when the signed-in user holds the platform_admin role key. */
export function useIsPlatformAdmin(): boolean {
  const { user } = useAuth();
  return !!user?.roleKeys.includes('platform_admin');
}

/** Field -> messages, as the platform forms render them under each input. */
export type FieldErrors = Record<string, string[]>;

/** Codes the platform returns when the caller is not a tenant-less platform admin. */
const PLATFORM_FORBIDDEN_CODES = new Set(['platform_only', 'platform_principal_required']);

/** Domain error codes that belong to a single form field. */
const CODE_FIELDS: Record<string, string> = {
  invalid_name: 'name',
  invalid_slug: 'slug',
  event_type_slug_taken: 'slug',
  event_type_slug_immutable: 'slug',
  event_category_slug_taken: 'slug',
  invalid_description: 'description',
  unknown_event_category: 'category_id',
  invalid_property_schema: 'property_schema',
};

export interface SplitErrors {
  /** Errors to render under their field. */
  fields: FieldErrors;
  /** Whatever could not be attributed to a known field. */
  form: string | null;
}

/**
 * Splits an API failure into per-field messages (problem+json `errors` and
 * field-specific codes) and a form-level message for the rest.
 */
export function splitApiErrors(err: unknown, knownFields: readonly string[], fallback: string): SplitErrors {
  if (!(err instanceof ApiRequestError)) {
    return { fields: {}, form: err instanceof Error ? err.message : fallback };
  }
  if (err.code && PLATFORM_FORBIDDEN_CODES.has(err.code)) {
    return { fields: {}, form: platformForbiddenMessage() };
  }
  const fields: FieldErrors = {};
  const rest: string[] = [];
  for (const [field, messages] of Object.entries(err.validationErrors ?? {})) {
    if (knownFields.includes(field)) fields[field] = messages;
    else rest.push(`${field}: ${messages.join(', ')}`);
  }
  const codeField = err.code ? CODE_FIELDS[err.code] : undefined;
  if (codeField && knownFields.includes(codeField) && !fields[codeField]) {
    fields[codeField] = [err.message];
  } else if (!err.validationErrors) {
    rest.push(err.message);
  }
  return { fields, form: rest.length ? rest.join('\n') : null };
}

/** A one-line message for toasts and error cards. */
export function apiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError) {
    if (err.code && PLATFORM_FORBIDDEN_CODES.has(err.code)) return platformForbiddenMessage();
    if (err.validationErrors) {
      return Object.entries(err.validationErrors)
        .map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
        .join('\n');
    }
    return err.message;
  }
  return err instanceof Error ? err.message : fallback;
}

export function platformForbiddenMessage(): string {
  return 'The platform API only accepts platform admins signed in without a tenant.';
}

export function formatDate(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export function formatDateTime(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

const SCHEMA_PROPERTY_TYPES = ['string', 'number', 'integer', 'boolean', 'object', 'array', 'null'];
const MAX_SCHEMA_PROPERTIES = 100;

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

export interface SchemaParseResult {
  ok: boolean;
  /** The parsed schema when ok (null for empty text). */
  schema: EventPropertySchema | null;
  /** Every problem found; empty when ok. */
  errors: string[];
}

/**
 * Parses the property_schema editor text and applies the same JSON Schema
 * subset checks as the API (eventcatalog/internal/domain/schema.go), listing
 * every problem instead of stopping at the first. Empty text means no schema.
 */
export function parsePropertySchema(text: string): SchemaParseResult {
  if (!text.trim()) return { ok: true, schema: null, errors: [] };
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    return { ok: false, schema: null, errors: [`Invalid JSON: ${e instanceof Error ? e.message : 'parse error'}`] };
  }
  if (!isPlainObject(parsed)) {
    return { ok: false, schema: null, errors: ['The schema must be a JSON object.'] };
  }

  const errors: string[] = [];
  if ('type' in parsed && parsed.type !== 'object') {
    errors.push('"type" must be "object".');
  }

  let props: Record<string, unknown> | null = null;
  if ('properties' in parsed) {
    if (!isPlainObject(parsed.properties)) {
      errors.push('"properties" must be an object.');
    } else {
      props = parsed.properties;
      const names = Object.keys(props);
      if (names.length > MAX_SCHEMA_PROPERTIES) {
        errors.push(`At most ${MAX_SCHEMA_PROPERTIES} properties are allowed.`);
      }
      for (const name of names) {
        const def = props[name];
        if (!isPlainObject(def)) {
          errors.push(`Property "${name}" must be an object.`);
          continue;
        }
        if ('type' in def && (typeof def.type !== 'string' || !SCHEMA_PROPERTY_TYPES.includes(def.type))) {
          errors.push(`Property "${name}" has an unsupported type (use ${SCHEMA_PROPERTY_TYPES.join(', ')}).`);
        }
      }
    }
  }

  if ('required' in parsed) {
    const required = parsed.required;
    if (!Array.isArray(required) || required.some(item => typeof item !== 'string')) {
      errors.push('"required" must be an array of property names.');
    } else {
      for (const name of required as string[]) {
        if (!props || !(name in props)) {
          errors.push(`Required property "${name}" is not declared in "properties".`);
        }
      }
    }
  }

  return errors.length
    ? { ok: false, schema: null, errors }
    : { ok: true, schema: parsed as EventPropertySchema, errors: [] };
}
