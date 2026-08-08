# minecraft-map

(Japanase)
`client.jar` やリソースパック等のアセットからカラーパレットを自動生成し、Minecraft の地図風の 2D ワールドマップを描画するスタンドアロン型ソフトウェア。

(English)
A standalone software application that automatically generates a color palette from assets such as `client.jar` and resource packs, and renders a 2D world map in the style of a Minecraft map.

## Example image

Minecraft Vanilla v1.21.8 で自然生成されたワールドデータで出力したイメージです  
This image was generated using naturally generated world data from Minecraft Vanilla v1.21.8.

- 地図情報 / Map information
  - リージョンファイル数 / Region files: 104 (range: X[-7..4], Z[-5..4])
  - 画像サイズ / Image size: 6144x5120 px (1block=1px)
  - 生成時間 / Generation time: 42 seconds
- 色について / Color information
  - 青紫色: 未読み込みチャンク
  - Blue-purple: unloaded chunks
  - 赤紫色: 未生成チャンク
  - Read-purple: ungenerated chunks

![](./example_map.png)

## Development Environment

テスト済み / Tested with:

- Minecraft Java Edition: 1.10.2, 1.21.8, 1.21.11, 26.1
- Go: 1.26.4

多分動作する / Should work on:

- Minecraft Java Edition: 1.5+
  (ブロック ID 仕様の都合上、v1.13+のワールドデータでの利用を推奨 / Recommended v1.13+ for full block-state support)
- Go: 1.21+

## Getting Started

### 1. コンフィグファイルの作成 / Creating a configuration file

1. `config_example.json`をコピーし、`config.json`にリネームする \
   Copy `config_example.json` and rename it to `config.json`
2. 以下の例に合わせて内容を書き換える \
   Rewrite the content to match the following example

```jsonc
{
  // ワールドのリージョンファイル(.mca) のディレクトリ / Directory containing the world's region file (.mca)
  "regionDir": "./world/region",
  // アセット読み込み元のリスト(.jar / .zip / 展開済みディレクトリを指定可能) / List of asset sources (.jar / .zip / extracted directory)
  // 配列の先頭が初期値(ベース)、末尾ほど優先度が高く、同じパスのファイルは後方のソースで上書きされる
  // The first entry is the base (default). Later entries take priority and overwrite files at the same path from earlier entries.
  "resources": [
    "~/.minecraft/versions/26.1.1/26.1.1.jar",
    "./resources/some_resourcepack.zip",
    "./resources/my_override_pack"
  ],
  // カラーパレットの保存ファイル名 / Color palette file name
  "mapColorJson": "./map_color.json",
  "export": {
    // 出力先のディレクトリ / Output directory
    "dir": "./export",
    // 高低差による影の描画 / Rendering shadows based on elevation differences
    "shading": true,
    // リージョンファイル(.mca)単位での出力の有効化 /  Enable output by region file (.mca)
    "byRegion": false,
    // PNGファイルの出力モード / PNG file output mode
    // "default" | "compression"
    "mode": "default"
  },
  "fallbackColor": {
    // 未生成チャンクのレンダリングカラー / Rendering color for ungenerated chunks
    // [R,G,B,A]
    "ungenerated": [106, 90, 205, 255],
    // リージョン/チャンクの読み込み(I/O・解凍)失敗時のレンダリングカラー / Rendering color for region/chunk read (I/O, decompress) failures
    // [R,G,B,A]
    "readError": [255, 0, 0, 255],
    // チャンクNBTのパース失敗(データ破損等)時のレンダリングカラー / Rendering color for chunk NBT parse failures (corrupted data, etc.)
    // [R,G,B,A]
    "parseError": [255, 140, 0, 255],
    // 全ブロックがsuppressBlocks対象(air等)だった、正常な空洞(奈落等)のレンダリングカラー
    // 既定は完全透明(alpha=0)にし、他のエラー色と混同しないようにしている
    // Rendering color for a column where every block is a suppressed block (e.g. air) — a genuine void, not an error.
    // Defaults to fully transparent so it's never confused with the error colors above.
    // [R,G,B,A]
    "void": [0, 0, 0, 0],
    // ブロックは検出できたが mapColorJson に該当する色情報が無かった際のレンダリングカラー
    // Rendering color for a block that was detected but has no matching entry in mapColorJson
    // [R,G,B,A]
    "missingColor": [255, 255, 0, 255],
    // 上記いずれにも該当しない、予期しない状態のレンダリングカラー(通常は発生しない)
    // Rendering color for any other unexpected state not covered above (should not normally occur)
    // [R,G,B,A]
    "other": [255, 0, 255, 255]
  },
  // ブロックカラーが見つからない際のフォールバック設定 / Fallback settings when block colors cannot be found
  // フォールバックは `mapColorJson` を参照する / The fallback refers to `mapColorJson`
  "fallbackBlocks": {
    // "blockID": "fallback blockID"
    "snow": "snow_height2"
  },
  // マップ表示の除外設定にするブロックIDのリスト / List of block IDs to exclude from map display
  "suppressBlocks": ["air", "cave_air", "void_air", "light", "barrier", "structure_void"]
}
```

`resources` に指定できるもの / What can be listed in `resources`:

- `client.jar`（バニラのアセット。基本的に一番最初に指定する / vanilla assets, normally listed first）
- リソースパックの `.zip` ファイル / resource pack `.zip` files
- リソースパックや MOD アセットを展開したディレクトリ / extracted resource pack or mod asset directories

各ソースは `assets/<namespace>/...` を自動的に走査するため、`minecraft` 以外の namespace（MOD 等）も自動で検出されます。  
Each source is automatically scanned under `assets/<namespace>/...`, so namespaces other than `minecraft` (e.g. mods) are detected automatically as well.

### 2. カラーパレットの生成 / Generate color palette

以下のコマンドを実行する  
Run the following command

```bash
go run . color
```

#### `export.mode` について / About `export.mode`

- **`default`**: 可逆圧縮のフルカラー(RGBA)PNG として出力します。色の劣化や透過の欠落は一切発生しませんが、ファイルサイズは大きくなります。  
  Outputs a fully lossless full-color (RGBA) PNG. There is no color degradation or loss of transparency, but the file size is larger.

- **`compression`**: ImageMagick の `-quantize YUV +dither -colors 256 -type Palette` に相当する疑似パレット化を行い、最大 256 色のパレット PNG(color-type 3)として出力します。ファイルサイズは大幅に削減されますが、以下の制約があります。  
  Performs pseudo palette quantization equivalent to ImageMagick's `-quantize YUV +dither -colors 256 -type Palette`, producing a palette PNG (color-type 3) with at most 256 colors. This greatly reduces file size, but comes with the following trade-offs.

  - **色味の変動が発生します(不可逆)** / **Color shift occurs (lossy)**  
    出現頻度の高い上位 256 色のみがパレットに残り、それ以外の色は最も近い色に置き換えられます。特に `shading` を有効にしている場合、影の濃淡のグラデーションが 256 色に集約しきれず、階調(バンディング)が視認できる場合があります。  
    Only the 256 most frequent colors are kept in the palette; every other color is replaced by its nearest match. Especially when `shading` is enabled, the shading gradient may not fit entirely within 256 colors, and visible banding can occur.

  - **透過(アルファ)は対応していますが、パレット枠は保証されません** / **Transparency (alpha) is supported, but a palette slot is not guaranteed**  
    PNG Paletted 形式(`tRNS`チャンク)による透過自体には対応しています。ただし、パレットの 256 色は画像中の出現頻度が高い順に機械的に選出されるだけで、`fallbackColor.void`(既定は完全透明)のような透明ピクセルを優先的にパレットへ確保する処理は行われません。そのため、透明ピクセルの出現数が他の色に比べて少ない画像では、透明色がそもそもパレットに含まれず、本来透明であるべき箇所が最も近い不透明色に置き換わってしまう場合があります。また、水やガラス等の半透明ブロックが多用され、かつ完全透明(alpha=0)ピクセルの絶対量が少ない画像では、パレット中で透明・半透明色に割ける枠はごくわずかです。そのため、深度や重なりによって生じる半透明色の濃淡差(例: 深い水と浅い水の alpha 差)は限られた枠を奪い合う形になり、`default`モードに比べて階調が粗くなります。  
    Transparency via the PNG palette format's `tRNS` chunk is supported in principle. However, the 256 palette colors are chosen mechanically by pixel frequency alone, and there is no logic that reserves a slot for transparent pixels such as `fallbackColor.void` (transparent by default). As a result, if transparent pixels are rare compared to other colors in the image, the transparent color may not make it into the palette at all, and pixels that should be transparent can be replaced by the nearest opaque color instead. In addition, when semi-transparent blocks like water or glass are heavily used and truly transparent (alpha=0) pixels are rare, only a handful of palette slots end up available for transparent/semi-transparent colors. As a result, alpha gradations caused by depth or overlap (e.g. the alpha difference between deep and shallow water) compete for these limited slots and appear coarser than in `default` mode.

  - 色の正確性や透過の完全性を優先する場合は `default` を、ファイルサイズを優先する場合は `compression` を選択してください。  
    Choose `default` when color accuracy and full transparency matter most, and `compression` when file size is the priority.

### 3. ワールドマップの出力 / Generate world map

以下のコマンドを実行する
Run the following command

```bash
go run . generate
```

実行時のログに出る、`[WARN] ... missing color blocks ...`については、config.json の`fallbackBlocks` もしくは `suppressBlocks` に設定を追加すること  
Regarding the `[WARN] ... missing color blocks ...` message that appears in the runtime log, add a setting to either `fallbackBlocks` or `suppressBlocks` in `config.json`.