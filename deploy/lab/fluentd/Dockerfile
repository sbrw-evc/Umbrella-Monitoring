FROM fluent/fluentd:v1.19-debian-2
USER root
RUN gem install --no-document fluent-plugin-kafka fluent-plugin-opensearch \
 && gem sources --clear-all \
 && rm -rf /tmp/* /var/tmp/* /usr/lib/ruby/gems/*/cache/*.gem
COPY fluent.conf /fluentd/etc/fluent.conf
USER fluent
