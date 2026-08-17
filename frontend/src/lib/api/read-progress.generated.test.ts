import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OpenAPI, SessionsService } from "./generated/index";
import type {
  DbSession,
  DbSidebarSessionIndexRow,
  ServiceSessionDetail,
} from "./generated/index";
import type { Session, SidebarSessionIndexRow } from "./types.js";
import dbSessionSource from "./generated/models/DbSession.ts?raw";
import dbSidebarRowSource
  from "./generated/models/DbSidebarSessionIndexRow.ts?raw";
import serviceDetailSource
  from "./generated/models/ServiceSessionDetail.ts?raw";
import coreTypesSource from "./types/core.ts?raw";

/** The generator is the only writer of these model files; assert on their
 *  real contents so a hand-edit or a stale generator run is a test failure
 *  rather than drift discovered later in the browser. */
const GENERATED_MODELS: Array<[string, string]> = [
  ["DbSession", dbSessionSource],
  ["DbSidebarSessionIndexRow", dbSidebarRowSource],
  ["ServiceSessionDetail", serviceDetailSource],
];

let fetchMock: ReturnType<typeof vi.fn>;

function installFetchMock(body: unknown) {
  fetchMock = vi.fn(async () =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
}

describe("Phase 20 generated transcript revision projection", () => {
  beforeEach(() => {
    OpenAPI.BASE = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it.each(GENERATED_MODELS)(
    "%s declares transcript_revision as an optional string",
    (_model, source) => {
      expect(source).toContain("transcript_revision?: string;");
      expect(source).not.toContain("transcript_revision: string;");
    },
  );

  it("hand written core types mirror the generated field", () => {
    const occurrences =
      coreTypesSource.match(/transcript_revision\?:/gu) ?? [];

    // Session and SidebarSessionIndexRow both carry it.
    expect(occurrences).toHaveLength(2);
  });

  it("keeps the field on the detail response", async () => {
    installFetchMock({
      id: "s1",
      transcript_revision: "7",
    });

    const detail = await SessionsService.getApiV1SessionsId({
      id: "s1",
    }) as unknown as ServiceSessionDetail;

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(detail.transcript_revision).toBe("7");
  });

  it("keeps the field on the sidebar index rows", async () => {
    installFetchMock({
      sessions: [{ id: "s1", transcript_revision: "9" }],
      total: 1,
    });

    const index = await SessionsService.getApiV1SessionsSidebarIndex(
      {},
    ) as unknown as { sessions: DbSidebarSessionIndexRow[] };

    expect(index.sessions[0]?.transcript_revision).toBe("9");
  });

  it("types the field identically on both sides of the boundary", () => {
    // Compile-time coupling: `npm run check` fails if either the generated
    // models or the hand written core types drop the field.
    const generated: Pick<DbSession, "transcript_revision"> = {
      transcript_revision: "4",
    };
    const generatedRow: Pick<
      DbSidebarSessionIndexRow,
      "transcript_revision"
    > = { transcript_revision: "4" };
    const handWritten: Pick<Session, "transcript_revision"> = {
      transcript_revision: generated.transcript_revision,
    };
    const handWrittenRow: Pick<
      SidebarSessionIndexRow,
      "transcript_revision"
    > = { transcript_revision: generatedRow.transcript_revision };

    expect(handWritten.transcript_revision).toBe("4");
    expect(handWrittenRow.transcript_revision).toBe("4");
  });
});
