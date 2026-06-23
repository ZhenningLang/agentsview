package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Server) registerSkillRoutes() {
	group := newRouteGroup(s.api, "/api/v1/skills", "Skills")

	get(s, group, "", "List skills", s.humaListSkills)
	get(s, group, "/cost", "Skill static token cost", s.humaSkillCost)
	get(s, group, "/health", "Skill catalog health", s.humaSkillHealth)
	get(s, group, "/{name}", "Get one skill", s.humaGetSkill)
}

type skillsListInput struct {
	Domain string `query:"domain" doc:"Filter by domain"`
	Role   string `query:"role" doc:"Filter by role"`
}

type skillsListOutput struct {
	Skills []db.Skill `json:"skills"`
}

func (s *Server) humaListSkills(
	ctx context.Context, in *skillsListInput,
) (*jsonOutput[skillsListOutput], error) {
	skills, err := s.db.ListSkills(ctx, db.SkillFilter{
		Domain: in.Domain,
		Role:   in.Role,
	})
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if skills == nil {
		skills = []db.Skill{}
	}
	return &jsonOutput[skillsListOutput]{Body: skillsListOutput{Skills: skills}}, nil
}

type skillGetInput struct {
	Name string `path:"name" doc:"Skill name"`
}

func (s *Server) humaGetSkill(
	ctx context.Context, in *skillGetInput,
) (*jsonOutput[db.Skill], error) {
	skill, err := s.db.GetSkill(ctx, in.Name)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if skill == nil {
		return nil, apiError(http.StatusNotFound, "skill not found")
	}
	return &jsonOutput[db.Skill]{Body: *skill}, nil
}

func (s *Server) humaSkillCost(
	ctx context.Context, _ *struct{},
) (*jsonOutput[db.SkillTokenCostReport], error) {
	rep, err := s.db.GetSkillTokenCost(ctx)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	return &jsonOutput[db.SkillTokenCostReport]{Body: rep}, nil
}

type skillHealthInput struct {
	Skill     string `query:"skill" doc:"Filter by skill name"`
	CheckType string `query:"check_type" doc:"Filter by check type"`
	Severity  string `query:"severity" doc:"Filter by severity"`
}

func (s *Server) humaSkillHealth(
	ctx context.Context, in *skillHealthInput,
) (*jsonOutput[db.SkillHealthReport], error) {
	rep, err := s.db.GetSkillHealth(ctx, db.SkillHealthFilter{
		SkillName: in.Skill,
		CheckType: in.CheckType,
		Severity:  in.Severity,
	})
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if rep.Findings == nil {
		rep.Findings = []db.SkillHealth{}
	}
	return &jsonOutput[db.SkillHealthReport]{Body: rep}, nil
}
