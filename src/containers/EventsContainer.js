import React, { Component } from 'react';
import moment from 'moment';
import PropTypes from 'prop-types';
import { connect } from 'react-redux';
import Components from '../components';
import Actions from '../actions';
import Spinner from "react-spinkit";

const TimerangeSelector = Components.Misc.TimerangeSelector;

// EventsContainer Component
class EventsContainer extends Component {
  componentDidMount() {
    if (this.props.dates) {
      const dates = this.props.dates;
      this.props.getData(dates.startDate, dates.endDate);
    }
  }

  componentWillReceiveProps(nextProps) {
    if (nextProps.dates && (this.props.dates !== nextProps.dates || this.props.accounts !== nextProps.accounts))
      nextProps.getData(nextProps.dates.startDate, nextProps.dates.endDate);
  }

  formatEvents(events) {
    const abnormalsList = [];

    Object.keys(events).forEach((key) => {
      const event = events[key];
      const abnormals = event.filter((item) => (item.abnormal));
      abnormals.forEach((element) => {
        abnormalsList.push({element, key, event});
      });
    });

    abnormalsList.sort((a, b) => ((moment(a.element.date).isBefore(b.element.date)) ? 1 : -1));

    return abnormalsList.map((abnormal) => {
      const element = abnormal.element;
      const key = abnormal.key;
      const dataSet = abnormal.event;
      return (
        <div key={`${element.date}-${key}`}>
          <Components.Events.EventPanel
            dataSet={dataSet}
            abnormalElement={element}
            service={key}
          />
        </div>
      );
    });
  }

  render() {
    const loading = (!this.props.values.status ? (<Spinner className="spinner" name='circle'/>) : null);

    const error = (this.props.values.error ? ` (${this.props.values.error.message})` : null);
    const noEvent = (this.props.values.status && (!this.props.values.values || !Object.keys(this.props.values.values).length || error) ? <div className="alert alert-warning" role="alert">No event available{error}</div> : "");

    const timerange = (this.props.dates ?  (
      <TimerangeSelector
        startDate={this.props.dates.startDate}
        endDate={this.props.dates.endDate}
        setDatesFunc={this.props.setDates}
      />
    ) : null);

    let events = [];
    if (this.props.values && this.props.values.status && this.props.values.values)
      events = this.formatEvents(this.props.values.values);

    const spinnerAndError = (loading || noEvent ? (
      <div className="white-box">
        {loading}
        {noEvent}
      </div>
    ) : null);

    return (
      <div>
        <div className="row">
          <div className="col-md-12">
            <div className="white-box">
              <h3 className="white-box-title no-padding inline-block">
                <i className="fa fa-exclamation-triangle"></i>
                &nbsp;
                Events
              </h3>
              <div className="inline-block pull-right">
                {timerange}
              </div>
            </div>
          </div>
        </div>
        {spinnerAndError}
        {events}
      </div>
    );
  }

}

EventsContainer.propTypes = {
  dates: PropTypes.object.isRequired,
  values: PropTypes.object.isRequired,
  accounts: PropTypes.arrayOf(PropTypes.object),
  getData: PropTypes.func.isRequired,
  setDates: PropTypes.func.isRequired,
};

/* istanbul ignore next */
const mapStateToProps = ({aws, events}) => ({
  dates: events.dates,
  accounts: aws.accounts.selection,
  values: events.values,
});

/* istanbul ignore next */
const mapDispatchToProps = (dispatch) => ({
  getData: (begin, end) => {
    dispatch(Actions.Events.getData(begin, end));
  },
  setDates: (startDate, endDate) => {
    dispatch(Actions.Events.setDates(startDate, endDate));
  }
});

export default connect(mapStateToProps, mapDispatchToProps)(EventsContainer);
