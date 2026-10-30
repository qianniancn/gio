// SPDX-License-Identifier: Unlicense OR MIT

// Native text displays Chinese and Latin text shaped by the platform text
// engine selected by the default Gio text shaper.
package main

import (
	"fmt"
	"image/color"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	nativetext "gioui.org/text/native"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("Gio Native Text Samples"), app.Size(unit.Dp(900), unit.Dp(720)))
		th := material.NewTheme()
		th.Face = "Microsoft YaHei"
		th.Shaper = text.NewShaper(text.WithClearType(
			color.NRGBA{A: 0xff},
			color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		))
		var samples widget.List
		samples.List.Axis = layout.Vertical
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				paint.Fill(gtx.Ops, th.Palette.Bg)
				layout.Inset{Top: 18, Left: 24, Right: 24, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					status := "系统文本后端不可用，当前使用 Gio 回退"
					if nativetext.Shared().Available() {
						status = "系统文本后端已启用：Windows DirectWrite / macOS Core Text / Linux Pango"
						if nativetext.Shared().SubpixelEnabled() {
							status += " · ClearType 可用"
						} else {
							status += " · 当前系统使用灰度抗锯齿"
						}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.H5(th, "Gio 原生文字样张").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: 4}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Caption(th, status+"  ·  滚动查看字号与语言样张").Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return samples.Layout(gtx, 12, func(gtx layout.Context, index int) layout.Dimensions {
								gtx.Locale = system.Locale{Language: "zh-CN"}
								gap := layout.Spacer{Height: 14}
								switch index {
								case 0:
									return sampleHeading(gtx, th, "常用字号对比")
								case 1:
									return sampleSize(gtx, th, 12, "小字注释", "Caption  ·  English 0123456789")
								case 2:
									return sampleSize(gtx, th, 14, "紧凑正文", "Compact body  ·  English 0123456789")
								case 3:
									return sampleSize(gtx, th, 16, "默认正文", "Body  ·  English 0123456789")
								case 4:
									return sampleSize(gtx, th, 20, "强调正文", "Large body  ·  English 0123456789")
								case 5:
									return sampleSize(gtx, th, 28, "小标题", "Heading  ·  English 0123456789")
								case 6:
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(gap.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sampleHeading(gtx, th, "语言与字体回退") }))
								case 7:
									return material.Label(th, 18, "简体中文：你好，世界！字体回退、标点与数字 2026。\n繁體中文：你好，世界！字體回退與標點測試。").Layout(gtx)
								case 8:
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return latinLabel(th, 18, "English: The quick brown fox jumps over the lazy dog.").Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return latinLabel(th, 18, "Digits: 0123456789  $123.45  +1 (555) 012-3456").Layout(gtx)
										}),
									)
								case 9:
									return material.Label(th, 18, "日本語：こんにちは、世界。かな・カナ・漢字。\n한국어: 안녕하세요, 세계! 한글 글꼴 대체 확인.").Layout(gtx)
								case 10:
									gtx.Locale = system.Locale{Language: "ar", Direction: system.RTL}
									return material.Label(th, 18, "العربية: مرحباً بالعالم، اختبار النص والأرقام 12345.").Layout(gtx)
								default:
									return material.Label(th, 18, "Русский: Привет, мир! Ελληνικά: Γειά σου κόσμε!\nहिन्दी: नमस्ते दुनिया!  ภาษาไทย: สวัสดีชาวโลก").Layout(gtx)
								}
							})
						}),
					)
				})
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

func sampleHeading(gtx layout.Context, th *material.Theme, title string) layout.Dimensions {
	return material.Label(th, 16, title).Layout(gtx)
}

func sampleSize(gtx layout.Context, th *material.Theme, size int, chinese, latin string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return latinLabel(th, unit.Sp(size), fmt.Sprintf("%d sp", size)).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Label(th, unit.Sp(size), chinese).Layout(gtx)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return latinLabel(th, unit.Sp(size), latin).Layout(gtx)
		}),
	)
}

func latinLabel(th *material.Theme, size unit.Sp, value string) material.LabelStyle {
	return material.Label(th, size, value)
}
