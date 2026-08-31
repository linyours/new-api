package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

type createChannelTemplateFromChannelRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	NameSuffixLength int    `json:"name_suffix_length"`
}

type updateChannelTemplateRequest struct {
	Id               int     `json:"id"`
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	NameSuffixLength *int    `json:"name_suffix_length"`
	SourceChannelId  int     `json:"source_channel_id"`
}

type applyChannelTemplateRequest struct {
	Keys             string `json:"keys"`
	NameSuffixLength *int   `json:"name_suffix_length"`
}

type appliedChannelTemplateItem struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

func GetChannelTemplates(c *gin.Context) {
	templates, err := model.GetAllChannelTemplates()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, templates)
}

func GetChannelTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	template, err := model.GetChannelTemplateById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, template)
}

func CreateChannelTemplateFromChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var req createChannelTemplateFromChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(channelId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	template, err := buildChannelTemplateFromChannel(channel, req.Name, req.Description, req.NameSuffixLength)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := template.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.template.create", map[string]interface{}{
		"id":         template.Id,
		"name":       template.Name,
		"channel_id": channel.Id,
	})
	common.ApiSuccess(c, template)
}

func UpdateChannelTemplate(c *gin.Context) {
	var req updateChannelTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	template, err := model.GetChannelTemplateById(req.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	name := template.Name
	if req.Name != nil {
		name = *req.Name
	}
	normalizedName, err := model.NormalizeChannelTemplateName(name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	dup, err := model.IsChannelTemplateNameDuplicated(template.Id, normalizedName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if dup {
		common.ApiErrorMsg(c, "template name already exists")
		return
	}

	template.Name = normalizedName
	if req.Description != nil {
		template.Description = strings.TrimSpace(*req.Description)
	}
	if req.NameSuffixLength != nil {
		template.NameSuffixLength = *req.NameSuffixLength
	}
	if req.SourceChannelId > 0 {
		channel, err := model.GetChannelById(req.SourceChannelId, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		cfg, err := model.ChannelTemplateConfigFromChannel(channel)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if err := validateChannel(cfg.ToChannel(), false); err != nil {
			common.ApiError(c, err)
			return
		}
		encoded, err := cfg.MarshalConfig()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		template.Config = encoded
		template.ChannelType = channel.Type
	}

	if err := template.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.template.update", map[string]interface{}{
		"id":   template.Id,
		"name": template.Name,
	})
	common.ApiSuccess(c, template)
}

func DeleteChannelTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	template, err := model.GetChannelTemplateById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteChannelTemplateById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.template.delete", map[string]interface{}{
		"id":   id,
		"name": template.Name,
	})
	common.ApiSuccess(c, nil)
}

func ApplyChannelTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var req applyChannelTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	template, err := model.GetChannelTemplateById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg, err := template.ParsedConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	base := cfg.ToChannel()
	if err := validateChannel(base, false); err != nil {
		common.ApiError(c, err)
		return
	}

	keys, err := splitTemplateApplyKeys(base, req.Keys)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(keys) > model.MaxChannelTemplateApplyKeys {
		common.ApiErrorMsg(c, fmt.Sprintf("cannot apply more than %d keys at once", model.MaxChannelTemplateApplyKeys))
		return
	}

	suffixLength := template.NameSuffixLength
	if req.NameSuffixLength != nil {
		suffixLength = *req.NameSuffixLength
	}
	suffixLength = model.NormalizeChannelTemplateNameSuffixLength(suffixLength)

	reserved := make(map[string]struct{}, len(keys))
	channels := make([]model.Channel, 0, len(keys))
	now := common.GetTimestamp()
	operatorId := c.GetInt("id")
	for _, key := range keys {
		channel := *base
		channel.Key = key
		channel.CreatedTime = now
		channel.CreatedBy = operatorId
		channel.UpdatedBy = operatorId
		channel.Name, err = model.AllocateUniqueChannelName(
			model.BuildChannelTemplateChannelName(template.Name, key, suffixLength),
			reserved,
		)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if err := validateChannel(&channel, true); err != nil {
			common.ApiError(c, err)
			return
		}
		channels = append(channels, channel)
	}

	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := attachInsertedChannelIDs(channels); err != nil {
		common.ApiError(c, err)
		return
	}

	created := make([]appliedChannelTemplateItem, 0, len(channels))
	createdIds := make([]int, 0, len(channels))
	for i := range channels {
		createdIds = append(createdIds, channels[i].Id)
		created = append(created, appliedChannelTemplateItem{
			Id:   channels[i].Id,
			Name: channels[i].Name,
		})
	}

	if len(cfg.ModelRpmLimits) > 0 {
		for _, channelId := range createdIds {
			if err := model.ApplyChannelKeyModelRpmLimits(channelId, cfg.ModelRpmLimits); err != nil {
				if _, delErr := model.BatchDeleteChannels(createdIds); delErr != nil {
					common.SysError("failed to roll back template-created channels: " + delErr.Error())
				}
				common.ApiError(c, err)
				return
			}
		}
	}

	model.InitChannelCache()
	recordManageAudit(c, "channel.template.apply", map[string]interface{}{
		"id":    template.Id,
		"name":  template.Name,
		"count": len(created),
	})
	common.ApiSuccess(c, gin.H{
		"count":    len(created),
		"channels": created,
	})
}

func buildChannelTemplateFromChannel(channel *model.Channel, name, description string, suffixLength int) (*model.ChannelTemplate, error) {
	if channel == nil {
		return nil, errors.New("channel cannot be empty")
	}
	if strings.TrimSpace(name) == "" {
		name = channel.Name
	}
	normalizedName, err := model.NormalizeChannelTemplateName(name)
	if err != nil {
		return nil, err
	}
	dup, err := model.IsChannelTemplateNameDuplicated(0, normalizedName)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errors.New("template name already exists")
	}
	cfg, err := model.ChannelTemplateConfigFromChannel(channel)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(cfg.ToChannel(), false); err != nil {
		return nil, err
	}
	encoded, err := cfg.MarshalConfig()
	if err != nil {
		return nil, err
	}
	return &model.ChannelTemplate{
		Name:             normalizedName,
		Description:      strings.TrimSpace(description),
		ChannelType:      channel.Type,
		Config:           encoded,
		NameSuffixLength: model.NormalizeChannelTemplateNameSuffixLength(suffixLength),
	}, nil
}

func attachInsertedChannelIDs(channels []model.Channel) error {
	if len(channels) == 0 {
		return nil
	}
	names := make([]string, 0, len(channels))
	for i := range channels {
		if channels[i].Id > 0 {
			continue
		}
		if channels[i].Name == "" {
			return errors.New("created channel is missing a name")
		}
		names = append(names, channels[i].Name)
	}
	if len(names) == 0 {
		return nil
	}
	var stored []model.Channel
	if err := model.DB.Select("id", "name").Where("name IN ?", names).Find(&stored).Error; err != nil {
		return err
	}
	idsByName := make(map[string]int, len(stored))
	for _, channel := range stored {
		idsByName[channel.Name] = channel.Id
	}
	for i := range channels {
		if channels[i].Id > 0 {
			continue
		}
		id, ok := idsByName[channels[i].Name]
		if !ok || id <= 0 {
			return fmt.Errorf("failed to load created channel %s", channels[i].Name)
		}
		channels[i].Id = id
	}
	return nil
}

func splitTemplateApplyKeys(channel *model.Channel, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("keys cannot be empty")
	}
	if channel != nil &&
		channel.Type == constant.ChannelTypeVertexAi &&
		channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
		return getVertexArrayKeys(raw)
	}
	keys := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		keys = append(keys, line)
	}
	if len(keys) == 0 {
		return nil, errors.New("keys cannot be empty")
	}
	return keys, nil
}
