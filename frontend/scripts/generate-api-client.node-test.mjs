import assert from "node:assert/strict";
import test from "node:test";

import { normalizeNullableSchema } from "./generate-api-client.mjs";

test("normalizeNullableSchema converts OpenAPI 3.1 nullable arrays without losing items", () => {
  const spec = {
    components: {
      schemas: {
        UsageSummaryResponse: {
          type: "object",
          properties: {
            daily: {
              type: ["array", "null"],
              items: { $ref: "#/components/schemas/DbDailyUsageEntry" },
            },
            unpricedModels: {
              type: ["array", "null"],
              items: { type: "string" },
            },
            label: { type: ["string", "null"] },
          },
        },
      },
    },
  };

  normalizeNullableSchema(spec);

  const props = spec.components.schemas.UsageSummaryResponse.properties;
  assert.deepEqual(props.daily, {
    type: "array",
    nullable: true,
    items: { $ref: "#/components/schemas/DbDailyUsageEntry" },
  });
  assert.deepEqual(props.unpricedModels, {
    type: "array",
    nullable: true,
    items: { type: "string" },
  });
  assert.deepEqual(props.label, {
    type: "string",
    nullable: true,
  });

  normalizeNullableSchema(spec);
  assert.deepEqual(props.daily, {
    type: "array",
    nullable: true,
    items: { $ref: "#/components/schemas/DbDailyUsageEntry" },
  });
});

test("normalizeNullableSchema leaves multi-type unions unchanged", () => {
  const schema = { type: ["string", "number", "null"] };

  normalizeNullableSchema(schema);

  assert.deepEqual(schema, { type: ["string", "number", "null"] });
});
