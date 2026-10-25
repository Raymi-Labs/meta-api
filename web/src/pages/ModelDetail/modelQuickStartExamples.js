/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import {
  buildDocsRequest,
  buildDocsRequestData,
  getDocsBaseUrl,
} from '../Docs/runtimeDocs.js';

export const MODEL_EXAMPLE_LANGUAGES = [
  'Python',
  'TypeScript',
  'Go',
  'Java',
  'C#',
  'Ruby',
  'cURL',
];

const PROTOCOLS = [
  ['openai', 'Chat Completions'],
  ['openai-response', 'Responses'],
  ['anthropic', 'Messages'],
  ['gemini', 'Gemini'],
  ['image-generation', 'Images'],
];

// Generated snippets contain administrator-supplied model names. Always render
// these as string literals, never as identifiers, interpolation or source code.
const quote = (value) =>
  JSON.stringify(value)
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029');
const rubyQuote = (value) => quote(value).replace(/#(?=[{$@])/g, '\\#');

function nativeLiteral(value, language, depth = 0) {
  if (value === null) return language === 'Python' ? 'None' : 'nil';
  if (typeof value === 'string') {
    return language === 'Ruby' ? rubyQuote(value) : quote(value);
  }
  if (typeof value === 'boolean' && language === 'Python') {
    return value ? 'True' : 'False';
  }
  if (typeof value !== 'object') return String(value);
  const indent = '  '.repeat(depth + 1);
  const closeIndent = '  '.repeat(depth);
  if (Array.isArray(value)) {
    return `[\n${value.map((item) => indent + nativeLiteral(item, language, depth + 1)).join(',\n')}\n${closeIndent}]`;
  }
  const delimiter = language === 'Ruby' ? ' => ' : ': ';
  return `{\n${Object.entries(value)
    .map(
      ([key, item]) =>
        indent +
        nativeLiteral(key, language) +
        delimiter +
        nativeLiteral(item, language, depth + 1),
    )
    .join(',\n')}\n${closeIndent}}`;
}

function requestHeaders(request) {
  return Object.fromEntries(
    [...request.headers, 'Content-Type: application/json'].map((header) => {
      const colon = header.indexOf(':');
      return [
        header.slice(0, colon),
        header
          .slice(colon + 1)
          .trim()
          .replace('$NAN_API_KEY', 'YOUR_API_KEY'),
      ];
    }),
  );
}

function renderRequest(request, language) {
  const { url, body } = request;
  const headers = requestHeaders(request);
  const bodyJSON = JSON.stringify(body);
  const jsonString = quote(bodyJSON);
  switch (language) {
    case 'Python':
      return `import requests

url = ${quote(url)}
headers = ${nativeLiteral(headers, 'Python')}
payload = ${nativeLiteral(body, 'Python')}

response = requests.post(url, headers=headers, json=payload, timeout=120)
response.raise_for_status()
print(response.json())`;
    case 'TypeScript':
      return `const response = await fetch(${quote(url)}, {
  method: "POST",
  headers: ${JSON.stringify(headers, null, 2).replace(/\n/g, '\n  ')},
  body: JSON.stringify(${JSON.stringify(body, null, 2).replace(/\n/g, '\n  ')}),
});

if (!response.ok) {
  throw new Error(await response.text());
}
console.log(await response.json());`;
    case 'Go':
      return `package main

import (
    "fmt"
    "io"
    "log"
    "net/http"
    "strings"
    "time"
)

func main() {
    payload := strings.NewReader(${jsonString})
    req, err := http.NewRequest("POST", ${quote(url)}, payload)
    if err != nil {
        log.Fatal(err)
    }
${Object.entries(headers)
  .map(([key, value]) => `    req.Header.Set(${quote(key)}, ${quote(value)})`)
  .join('\n')}

    client := &http.Client{Timeout: 120 * time.Second}
    response, err := client.Do(req)
    if err != nil {
        log.Fatal(err)
    }
    defer response.Body.Close()
    body, err := io.ReadAll(response.Body)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(body))
}`;
    case 'Java':
      return `import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

public class Main {
    public static void main(String[] args) throws Exception {
        String payload = ${jsonString};
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(${quote(url)}))
            .timeout(Duration.ofSeconds(120))
${Object.entries(headers)
  .map(([key, value]) => `            .header(${quote(key)}, ${quote(value)})`)
  .join('\n')}
            .POST(HttpRequest.BodyPublishers.ofString(payload))
            .build();
        HttpClient client = HttpClient.newHttpClient();
        HttpResponse<String> response = client.send(
            request, HttpResponse.BodyHandlers.ofString());
        System.out.println(response.body());
    }
}`;
    case 'C#':
      return `using System;
using System.Net.Http;
using System.Text;

using var client = new HttpClient();
client.Timeout = TimeSpan.FromSeconds(120);
using var request = new HttpRequestMessage(HttpMethod.Post, ${quote(url)});
${Object.entries(headers)
  .filter(([key]) => key !== 'Content-Type')
  .map(([key, value]) => `request.Headers.Add(${quote(key)}, ${quote(value)});`)
  .join('\n')}
string payload = ${jsonString};
request.Content = new StringContent(payload, Encoding.UTF8, "application/json");

using var response = await client.SendAsync(request);
response.EnsureSuccessStatusCode();
Console.WriteLine(await response.Content.ReadAsStringAsync());`;
    case 'Ruby':
      return `require "net/http"
require "json"
require "uri"

uri = URI(${rubyQuote(url)})
request = Net::HTTP::Post.new(uri)
${Object.entries(headers)
  .map(([key, value]) => `request[${rubyQuote(key)}] = ${rubyQuote(value)}`)
  .join('\n')}
payload = ${nativeLiteral(body, 'Ruby')}
request.body = JSON.generate(payload)

response = Net::HTTP.start(
  uri.hostname, uri.port,
  use_ssl: uri.scheme == "https", read_timeout: 120
) { |http| http.request(request) }
puts response.body`;
    default:
      return '';
  }
}

// Match the selected model's declared protocols. A model name alone never
// establishes which request formats its available channels support.
export function getModelQuickStartExamples(
  modelData,
  serverAddress,
  origin,
  language = 'cURL',
) {
  if (
    !MODEL_EXAMPLE_LANGUAGES.includes(language) ||
    typeof modelData?.model_name !== 'string' ||
    !modelData.model_name.trim() ||
    !Array.isArray(modelData.supported_endpoint_types)
  ) {
    return [];
  }
  const baseUrl = getDocsBaseUrl(serverAddress, origin);
  if (!baseUrl) return [];

  return PROTOCOLS.filter(([endpoint]) =>
    modelData.supported_endpoint_types.includes(endpoint),
  ).map(([endpoint, label]) => {
    const request = buildDocsRequestData(modelData, endpoint, baseUrl);
    const command = buildDocsRequest(modelData, endpoint, baseUrl)
      .replace(/^```bash\n/, '')
      .replace(/\n```$/, '');
    return {
      endpoint,
      label,
      code:
        language === 'cURL'
          ? `export NAN_API_KEY="YOUR_API_KEY"\n\n${command}`
          : renderRequest(request, language),
    };
  });
}
