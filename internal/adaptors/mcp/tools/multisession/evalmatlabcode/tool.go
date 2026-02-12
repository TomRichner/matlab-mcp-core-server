// Copyright 2025-2026 The MathWorks, Inc.

package evalmatlabcode

import (
	"context"

	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/application/config"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/mcp/tools"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/mcp/tools/annotations"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/mcp/tools/basetool"
	"github.com/matlab/matlab-mcp-core-server/internal/adaptors/mcp/tools/utils/responseconverter"
	"github.com/matlab/matlab-mcp-core-server/internal/entities"
	"github.com/matlab/matlab-mcp-core-server/internal/messages"
	"github.com/matlab/matlab-mcp-core-server/internal/usecases/evalmatlabcode"
)

type ConfigFactory interface {
	Config() (config.Config, messages.Error)
}

type Usecase interface {
	Execute(ctx context.Context, sessionLogger entities.Logger, client entities.MATLABSessionClient, request evalmatlabcode.Args) (entities.EvalResponse, error)
}

type Tool struct {
	basetool.ToolWithUnstructuredContentOutput[Args]
}

func New(
	loggerFactory basetool.LoggerFactory,
	configFactory ConfigFactory,
	usecase Usecase,
	matlabManager entities.MATLABManager,
) *Tool {
	return &Tool{
		ToolWithUnstructuredContentOutput: basetool.NewToolWithUnstructuredContent(name, title, description, annotations.NewDestructiveAnnotations(), loggerFactory, Handler(configFactory, usecase, matlabManager)),
	}
}

func Handler(configFactory ConfigFactory, usecase Usecase, matlabManager entities.MATLABManager) basetool.HandlerWithUnstructuredContentOutput[Args] {
	return func(ctx context.Context, sessionLogger entities.Logger, inputs Args) (tools.RichContent, error) {
		sessionID := entities.SessionID(inputs.SessionID)

		sessionLogger = sessionLogger.With("session_id", sessionID)

		sessionLogger.Info("Executing Eval in MATLAB Session tool")
		defer sessionLogger.Info("Done - Executing Eval in MATLAB Session tool")

		client, err := matlabManager.GetMATLABSessionClient(ctx, sessionLogger, sessionID)
		if err != nil {
			return tools.RichContent{}, err
		}

		response, err := usecase.Execute(ctx, sessionLogger, client, evalmatlabcode.Args{
			Code:          inputs.Code,
			ProjectPath:   inputs.ProjectPath,
			CaptureOutput: true,
		})
		if err != nil {
			return tools.RichContent{}, err
		}

		return responseconverter.ConvertEvalResponseToRichContent(response), nil
	}
}
