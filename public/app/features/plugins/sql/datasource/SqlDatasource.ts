import { lastValueFrom, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';

import {
  DataFrame,
  DataFrameView,
  DataQuery,
  DataSourceInstanceSettings,
  DataSourceRef,
  MetricFindValue,
  ScopedVars,
  TimeRange,
} from '@grafana/data';
import {
  BackendDataSourceResponse,
  DataSourceWithBackend,
  FetchResponse,
  getBackendSrv,
  getTemplateSrv,
  TemplateSrv,
} from '@grafana/runtime';
import { toDataQueryResponse, toTestingStatus } from '@grafana/runtime/src/utils/queryResponse';
import { getTimeSrv } from 'app/features/dashboard/services/TimeSrv';

import { VariableWithMultiSupport } from '../../../variables/types';
import { getSearchFilterScopedVar, SearchFilterOptions } from '../../../variables/utils';
import { ResponseParser } from '../ResponseParser';
import { MACRO_NAMES } from '../constants';
import { DB, SQLQuery, SQLOptions, SqlQueryModel, QueryFormat } from '../types';

export abstract class SqlDatasource extends DataSourceWithBackend<SQLQuery, SQLOptions> {
  id: number;
  responseParser: ResponseParser;
  name: string;
  interval: string;
  db: DB;
  annotations = {};

  constructor(
    instanceSettings: DataSourceInstanceSettings<SQLOptions>,
    protected readonly templateSrv: TemplateSrv = getTemplateSrv()
  ) {
    super(instanceSettings);
    this.name = instanceSettings.name;
    this.responseParser = new ResponseParser();
    this.id = instanceSettings.id;
    const settingsData = instanceSettings.jsonData || {};
    this.interval = settingsData.timeInterval || '1m';
    this.db = this.getDB();
  }

  abstract getDB(dsID?: number): DB;

  abstract getQueryModel(target?: SQLQuery, templateSrv?: TemplateSrv, scopedVars?: ScopedVars): SqlQueryModel;

  getResponseParser() {
    return this.responseParser;
  }

  interpolateVariable = (value: string | string[] | number, variable: VariableWithMultiSupport) => {
    if (typeof value === 'string') {
      if (variable.multi || variable.includeAll) {
        return this.getQueryModel().quoteLiteral(value);
      } else {
        return String(value).replace(/'/g, "''");
      }
    }

    if (typeof value === 'number') {
      return value;
    }

    if (Array.isArray(value)) {
      const quotedValues = value.map((v) => this.getQueryModel().quoteLiteral(v));
      return quotedValues.join(',');
    }

    return value;
  };

  interpolateVariablesInQueries(queries: SQLQuery[], scopedVars: ScopedVars): SQLQuery[] {
    let expandedQueries = queries;
    if (queries && queries.length > 0) {
      expandedQueries = queries.map((query) => {
        const expandedQuery = {
          ...query,
          datasource: this.getRef(),
          rawSql: this.templateSrv.replace(query.rawSql, scopedVars, this.interpolateVariable),
          rawQuery: true,
        };
        return expandedQuery;
      });
    }
    return expandedQueries;
  }

  filterQuery(query: SQLQuery): boolean {
    return !query.hide;
  }

  applyTemplateVariables(
    target: SQLQuery,
    scopedVars: ScopedVars
  ): Record<string, string | DataSourceRef | SQLQuery['format']> {
    return {
      refId: target.refId,
      datasource: this.getRef(),
      rawSql: this.templateSrv.replace(target.rawSql, scopedVars, this.interpolateVariable),
      format: target.format,
    };
  }

  async metricFindQuery(query: string, optionalOptions?: MetricFindQueryOptions): Promise<MetricFindValue[]> {
    let refId = 'tempvar';
    if (optionalOptions && optionalOptions.variable && optionalOptions.variable.name) {
      refId = optionalOptions.variable.name;
    }

    const rawSql = this.templateSrv.replace(
      query,
      getSearchFilterScopedVar({ query, wildcardChar: '%', options: optionalOptions }),
      this.interpolateVariable
    );

    const interpolatedQuery: SQLQuery = {
      refId: refId,
      datasource: this.getRef(),
      rawSql,
      format: QueryFormat.Table,
    };

    const response = await this.runMetaQuery(interpolatedQuery, optionalOptions);
    return this.getResponseParser().transformMetricFindResponse(response);
  }

  async runSql<T>(query: string, options?: RunSQLOptions) {
    const frame = await this.runMetaQuery({ rawSql: query, format: QueryFormat.Table, refId: options?.refId }, options);
    return new DataFrameView<T>(frame);
  }

  private runMetaQuery(request: Partial<SQLQuery>, options?: MetricFindQueryOptions): Promise<DataFrame> {
    const range = getTimeSrv().timeRange();
    const refId = request.refId || 'meta';
    const queries: DataQuery[] = [{ ...request, datasource: request.datasource || this.getRef(), refId }];

    return lastValueFrom(
      getBackendSrv()
        .fetch<BackendDataSourceResponse>({
          url: '/api/ds/query',
          method: 'POST',
          data: {
            from: options?.range?.from.valueOf().toString() || range.from.valueOf().toString(),
            to: options?.range?.to.valueOf().toString() || range.to.valueOf().toString(),
            queries,
          },
          requestId: refId,
        })
        .pipe(
          map((res: FetchResponse<BackendDataSourceResponse>) => {
            const rsp = toDataQueryResponse(res, queries);
            return rsp.data[0];
          })
        )
    );
  }

  testDatasource(): Promise<{ status: string; message: string }> {
    const refId = 'A';
    return lastValueFrom(
      getBackendSrv()
        .fetch<BackendDataSourceResponse>({
          url: '/api/ds/query',
          method: 'POST',
          data: {
            from: '5m',
            to: 'now',
            queries: [
              {
                refId: refId,
                intervalMs: 1,
                maxDataPoints: 1,
                datasource: this.getRef(),
                datasourceId: this.id,
                rawSql: 'SELECT 1',
                format: 'table',
              },
            ],
          },
        })
        .pipe(
          map((r) => {
            const error = r.data.results[refId].error;
            if (error) {
              return { status: 'error', message: error };
            }
            return { status: 'success', message: 'Database Connection OK' };
          }),
          catchError((err) => {
            return of(toTestingStatus(err));
          })
        )
    );
  }

  targetContainsTemplate(target: SQLQuery) {
    let queryWithoutMacros = target.rawSql;
    MACRO_NAMES.forEach((value) => {
      queryWithoutMacros = queryWithoutMacros?.replace(value, '') || '';
    });
    return this.templateSrv.containsTemplate(queryWithoutMacros);
  }
}

interface RunSQLOptions extends MetricFindQueryOptions {
  refId?: string;
}

interface MetricFindQueryOptions extends SearchFilterOptions {
  range?: TimeRange;
  variable?: VariableWithMultiSupport;
}
